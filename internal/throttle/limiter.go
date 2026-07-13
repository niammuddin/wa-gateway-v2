package throttle

import (
	"context"
	"database/sql"
	"math/rand"
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
	for {
		var interval, jitter, max int
		var throttledUntil sql.NullTime
		if err := l.db.QueryRowContext(ctx, `SELECT min_interval_ms,jitter_ms,max_messages_per_minute,throttled_until FROM sessions WHERE session_id=$1`, sessionID).Scan(&interval, &jitter, &max, &throttledUntil); err != nil {
			return err
		}
		now := time.Now()
		wait := time.Duration(0)
		if throttledUntil.Valid && throttledUntil.Time.After(now) {
			wait = throttledUntil.Time.Sub(now)
		}
		l.mu.Lock()
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
		intervalWait := time.Duration(interval) * time.Millisecond
		if jitter > 0 {
			intervalWait += time.Duration(rand.Intn(jitter+1)) * time.Millisecond
		}
		if !last.IsZero() && intervalWait > now.Sub(last) {
			if candidate := intervalWait - now.Sub(last); candidate > wait {
				wait = candidate
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

func (l *Limiter) RecordFailure(ctx context.Context, sessionID string) error {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var count, threshold, pause int
	var windowStarted sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT failure_count,failure_threshold,pause_duration_ms,failure_window_started_at FROM sessions WHERE session_id=$1 FOR UPDATE`, sessionID).Scan(&count, &threshold, &pause, &windowStarted); err != nil {
		return err
	}
	now := time.Now().UTC()
	if !windowStarted.Valid || now.Sub(windowStarted.Time) >= time.Minute {
		count = 1
		windowStarted = sql.NullTime{Time: now, Valid: true}
	} else {
		count++
	}
	var throttledUntil any
	if threshold > 0 && count >= threshold && pause > 0 {
		throttledUntil = now.Add(time.Duration(pause) * time.Millisecond)
	}
	_, err = tx.ExecContext(ctx, `UPDATE sessions SET failure_count=$2,failure_window_started_at=$3,throttled_until=COALESCE($4,throttled_until),updated_at=now() WHERE session_id=$1`, sessionID, count, windowStarted.Time, throttledUntil)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (l *Limiter) RecordSuccess(ctx context.Context, sessionID string) error {
	_, err := l.db.ExecContext(ctx, `UPDATE sessions SET failure_count=0,failure_window_started_at=NULL,throttled_until=NULL,updated_at=now() WHERE session_id=$1`, sessionID)
	return err
}
