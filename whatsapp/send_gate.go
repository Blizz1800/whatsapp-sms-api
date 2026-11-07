package whatsapp

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

type SendGate struct {
	mu       sync.Mutex
	interval time.Duration
	jitter   time.Duration
	next     time.Time
	now      func() time.Time
	sleep    func(context.Context, time.Duration) error
}

func NewSendGate(ratePerMinute int, jitter time.Duration) *SendGate {
	if ratePerMinute <= 0 {
		ratePerMinute = 20
	}
	return &SendGate{
		interval: time.Minute / time.Duration(ratePerMinute),
		jitter:   jitter,
		now:      time.Now,
		sleep:    sleepCtx,
	}
}

func (g *SendGate) Wait(ctx context.Context) error {
	for {
		g.mu.Lock()
		now := g.now()
		if !now.Before(g.next) {
			j := time.Duration(0)
			if g.jitter > 0 {
				j = time.Duration(rand.Int63n(int64(g.jitter) + 1))
			}
			g.next = now.Add(g.interval + j)
			g.mu.Unlock()
			return nil
		}
		wait := g.next.Sub(now)
		g.mu.Unlock()
		if err := g.sleep(ctx, wait); err != nil {
			return err
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
