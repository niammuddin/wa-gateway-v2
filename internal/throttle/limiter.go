package throttle

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

type Limiter struct {
	db     *sql.DB
	mu     sync.Mutex
	last   map[string]time.Time
	recent map[string][]time.Time
}

func New(db *sql.DB) *Limiter {
	return &Limiter{db: db, last: map[string]time.Time{}, recent: map[string][]time.Time{}}
}
func (l *Limiter) Wait(ctx context.Context, sessionID string) error {
	var interval, max int
	if err := l.db.QueryRowContext(ctx, `SELECT min_interval_ms,max_messages_per_minute FROM sessions WHERE session_id=$1`, sessionID).Scan(&interval, &max); err != nil {
		return err
	}
	for {
		l.mu.Lock()
		now := time.Now()
		last := l.last[sessionID]
		recent := l.recent[sessionID]
		cutoff := now.Add(-time.Minute)
		kept := recent[:0]
		for _, t := range recent {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		l.recent[sessionID] = kept
		wait := time.Duration(0)
		if !last.IsZero() {
			wait = time.Duration(interval)*time.Millisecond - now.Sub(last)
			if wait < 0 {
				wait = 0
			}
		}
		if max > 0 && len(kept) >= max && wait < time.Until(kept[0].Add(time.Minute)) {
			wait = time.Until(kept[0].Add(time.Minute))
		}
		if wait == 0 {
			l.last[sessionID] = now
			l.recent[sessionID] = append(kept, now)
			l.mu.Unlock()
			return nil
		}
		l.mu.Unlock()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
