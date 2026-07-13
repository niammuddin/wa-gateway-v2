package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/niammuddin/wa-gateway-v2/internal/store"
)

const MessageTask = "wa:send-message"

type MessageJob struct {
	MessageID string `json:"messageId"`
	SessionID string `json:"sessionId"`
	To        string `json:"to"`
	Type      string `json:"type"`
	Content   string `json:"content,omitempty"`
	URL       string `json:"url,omitempty"`
	Filename  string `json:"filename,omitempty"`
	MIMEType  string `json:"mimeType,omitempty"`
	Priority  int    `json:"priority,omitempty"`
	Delay     int    `json:"delay,omitempty"`
}

func queueName(priority int) string {
	switch {
	case priority > 0:
		return "messages_high"
	case priority < 0:
		return "messages_low"
	default:
		return "messages"
	}
}

func isSupportedMessageType(typ string) bool {
	switch typ {
	case "text", "image", "document", "pdf":
		return true
	default:
		return false
	}
}

func isMediaMessageType(typ string) bool {
	return typ == "image" || typ == "document" || typ == "pdf"
}

type Queue interface {
	Enqueue(context.Context, MessageJob, string) error
}

type Asynq struct{ client *asynq.Client }

func NewFromURL(raw string) (*Asynq, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	addr := u.Host
	if addr == "" {
		addr = strings.TrimPrefix(raw, "redis://")
	}
	return &Asynq{client: asynq.NewClient(asynq.RedisClientOpt{Addr: addr, DB: 0})}, nil
}

func (q *Asynq) Enqueue(ctx context.Context, job MessageJob, jobID string) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	opts := []asynq.Option{asynq.TaskID(jobID), asynq.Queue(queueName(job.Priority)), asynq.MaxRetry(3)}
	if job.Delay > 0 {
		opts = append(opts, asynq.ProcessIn(time.Duration(job.Delay)*time.Millisecond))
	}
	_, err = q.client.EnqueueContext(ctx, asynq.NewTask(MessageTask, payload), opts...)
	return err
}

func (q *Asynq) Close() error { return q.client.Close() }
func (q *Asynq) Recover(ctx context.Context, jobs []store.PendingJob) error {
	for _, pending := range jobs {
		var job MessageJob
		if err := json.Unmarshal(pending.Data, &job); err != nil {
			return err
		}
		if err := q.Enqueue(ctx, job, pending.JobID); err != nil {
			if strings.Contains(err.Error(), "task ID conflicts") {
				continue
			}
			return err
		}
	}
	return nil
}

type Nop struct{}

func (Nop) Enqueue(context.Context, MessageJob, string) error { return nil }

type Sender interface {
	SendText(context.Context, string, string) (string, error)
}
type MediaSender interface {
	SendMedia(context.Context, string, string, string, string, string) (string, error)
}
type MessageStore interface {
	UpdateMessageStatus(context.Context, string, string, string, string) error
	UpdateQueueJob(context.Context, string, string, string) error
}
type Dispatcher interface {
	Dispatch(context.Context, string, map[string]any) error
}
type Throttle interface {
	Wait(context.Context, string) error
	RecordFailure(context.Context, string) error
	RecordSuccess(context.Context, string) error
}
type Worker struct {
	server *asynq.Server
	mux    *asynq.ServeMux
}

func NewWorker(raw string, dataStore MessageStore, lookup func(string) (Sender, bool), dispatcher Dispatcher, limiter Throttle) (*Worker, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	addr := u.Host
	if addr == "" {
		addr = strings.TrimPrefix(raw, "redis://")
	}
	server := asynq.NewServer(asynq.RedisClientOpt{Addr: addr, DB: 0}, asynq.Config{Concurrency: 10, Queues: map[string]int{"messages_high": 10, "messages": 5, "messages_low": 1}})
	mux := asynq.NewServeMux()
	mux.HandleFunc(MessageTask, func(ctx context.Context, task *asynq.Task) error {
		var job MessageJob
		if err := json.Unmarshal(task.Payload(), &job); err != nil {
			return err
		}
		if limiter != nil {
			if err := limiter.Wait(ctx, job.SessionID); err != nil {
				return err
			}
		}
		recordFailure := func() {
			if limiter != nil {
				_ = limiter.RecordFailure(context.Background(), job.SessionID)
			}
		}
		recordSuccess := func() {
			if limiter != nil {
				_ = limiter.RecordSuccess(context.Background(), job.SessionID)
			}
		}
		sender, ok := lookup(job.SessionID)
		if !ok {
			recordFailure()
			_ = dataStore.UpdateMessageStatus(context.Background(), job.MessageID, "failed", "", "session client unavailable")
			_ = dataStore.UpdateQueueJob(context.Background(), job.MessageID, "failed", "session client unavailable")
			if dispatcher != nil {
				_ = dispatcher.Dispatch(context.Background(), "message.failed", map[string]any{"messageId": job.MessageID, "sessionId": job.SessionID, "to": job.To, "error": "session client unavailable"})
			}
			return fmt.Errorf("session client unavailable: %s", job.SessionID)
		}
		var waID string
		var err error
		switch {
		case job.Type == "text":
			waID, err = sender.SendText(ctx, job.To, job.Content)
		case isMediaMessageType(job.Type):
			if strings.TrimSpace(job.URL) == "" {
				err = fmt.Errorf("media URL is required for message type %s", job.Type)
				break
			}
			mediaSender, ok := sender.(MediaSender)
			if !ok {
				err = fmt.Errorf("media sending is not supported")
			} else {
				waID, err = mediaSender.SendMedia(ctx, job.To, job.Type, job.URL, job.Filename, job.Content)
			}
		case !isSupportedMessageType(job.Type):
			err = fmt.Errorf("unsupported message type %s", job.Type)
		}
		if err != nil {
			recordFailure()
			_ = dataStore.UpdateMessageStatus(context.Background(), job.MessageID, "failed", "", err.Error())
			_ = dataStore.UpdateQueueJob(context.Background(), job.MessageID, "failed", err.Error())
			if dispatcher != nil {
				_ = dispatcher.Dispatch(context.Background(), "message.failed", map[string]any{"messageId": job.MessageID, "sessionId": job.SessionID, "to": job.To, "error": err.Error()})
			}
			if strings.Contains(err.Error(), "WhatsApp reach-out timelock (error 463)") {
				// Error 463 is a WhatsApp server-side reach-out restriction.
				// Mark it failed without letting Asynq retry the same reach-out.
				return nil
			}
			return err
		}
		if err := dataStore.UpdateMessageStatus(ctx, job.MessageID, "sent", waID, ""); err != nil {
			return err
		}
		recordSuccess()
		_ = dataStore.UpdateQueueJob(context.Background(), job.MessageID, "completed", "")
		if dispatcher != nil {
			_ = dispatcher.Dispatch(ctx, "message.sent", map[string]any{"messageId": job.MessageID, "sessionId": job.SessionID, "to": job.To, "waMessageId": waID})
		}
		return nil
	})
	return &Worker{server: server, mux: mux}, nil
}
func (w *Worker) Run() error { return w.server.Run(w.mux) }
func (w *Worker) Shutdown()  { w.server.Shutdown() }
