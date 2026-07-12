package events

import (
	"context"
	"encoding/json"
	"sync"
)

type Event struct {
	Name string
	Data []byte
}
type Dispatcher interface {
	Dispatch(context.Context, string, map[string]any) error
}
type Multi struct{ Targets []Dispatcher }

func (m Multi) Dispatch(ctx context.Context, name string, payload map[string]any) error {
	var first error
	for _, target := range m.Targets {
		if target != nil {
			if err := target.Dispatch(ctx, name, payload); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

type Broker struct {
	mu          sync.RWMutex
	subscribers map[chan Event]struct{}
}

func NewBroker() *Broker { return &Broker{subscribers: map[chan Event]struct{}{}} }
func (b *Broker) Dispatch(_ context.Context, name string, payload map[string]any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subscribers {
		select {
		case ch <- Event{Name: name, Data: data}:
		default:
		}
	}
	return nil
}
func (b *Broker) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 32)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if _, ok := b.subscribers[ch]; ok {
			delete(b.subscribers, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
}
