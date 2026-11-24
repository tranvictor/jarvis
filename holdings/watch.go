package holdings

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/tranvictor/jarvis/kyber"
	"github.com/tranvictor/jarvis/ui"
	"golang.org/x/term"
)

// Watch redraws the book as prices move. It returns when ctx is cancelled.
func Watch(ctx context.Context, u ui.UI, l *Loader, b *Book) error {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("--watch needs a terminal")
	}
	fmt.Fprint(os.Stdout, "\033[?25l")
	defer fmt.Fprint(os.Stdout, "\033[?25h")

	b.Live = true
	prev := map[string]float64{}
	prevTotal := b.Total
	var havePrev bool
	paint := func() {
		b.ValueTicks = map[string]int{}
		b.TotalTick = 0
		if havePrev {
			for _, p := range b.Positions {
				if p.honeypot() || p.Value == nil {
					continue
				}
				old, ok := prev[p.Key()]
				if !ok {
					continue
				}
				if *p.Value > old+0.005 {
					b.ValueTicks[p.Key()] = 1
				} else if *p.Value < old-0.005 {
					b.ValueTicks[p.Key()] = -1
				}
			}
			if b.Total > prevTotal+0.005 {
				b.TotalTick = 1
			} else if b.Total < prevTotal-0.005 {
				b.TotalTick = -1
			}
		}
		fmt.Fprint(os.Stdout, "\033[H\033[2J")
		Render(u, b)
		prev = map[string]float64{}
		for _, p := range b.Positions {
			if p.honeypot() || p.Value == nil {
				continue
			}
			prev[p.Key()] = *p.Value
		}
		prevTotal = b.Total
		havePrev = true
	}

	var sock *kyber.MarketSocket
	var current []string
	backoff := time.Second
	var retryAt time.Time
	refresh := time.NewTicker(60 * time.Second)
	draw := time.NewTicker(200 * time.Millisecond)
	defer refresh.Stop()
	defer draw.Stop()
	defer func() {
		if sock != nil {
			sock.Close()
		}
	}()

	dirty := true
	resub := true

	for {
		if sock == nil && !time.Now().Before(retryAt) {
			s, err := l.Client.OpenMarketSocket(ctx)
			if err != nil {
				b.LiveNote = "reconnecting"
				retryAt = time.Now().Add(withJitter(backoff))
				backoff = nextBackoff(backoff)
				dirty = true
			} else {
				sock = s
				current = nil
				backoff = time.Second
				b.LiveNote = ""
				resub = true
			}
		}
		if sock != nil && resub {
			want := streamsFor(b)
			if !sameStreams(current, want) {
				if len(current) > 0 {
					_ = sock.Unsubscribe(current)
				}
				if err := sock.Subscribe(want); err != nil {
					b.LiveNote = "subscribe failed"
					sock.Close()
					sock = nil
					current = nil
					retryAt = time.Now().Add(withJitter(backoff))
					backoff = nextBackoff(backoff)
				} else {
					current = want
					b.LiveNote = ""
				}
			}
			resub = false
			dirty = true
		}

		var evCh <-chan kyber.MarketEvent
		if sock != nil {
			evCh = sock.Events
		}
		select {
		case <-ctx.Done():
			if !havePrev {
				paint()
			}
			return nil
		case ev, ok := <-evCh:
			if !ok || ev.Err != nil {
				if sock != nil {
					sock.Close()
				}
				sock = nil
				current = nil
				b.LiveNote = "reconnecting"
				retryAt = time.Now().Add(withJitter(backoff))
				backoff = nextBackoff(backoff)
				dirty = true
				continue
			}
			if b.Apply(ev) {
				dirty = true
			}
			if ev.Kind == "status" && ev.Degraded {
				_ = l.RefreshMarket(ctx, b)
				dirty = true
			}
		case <-refresh.C:
			if err := l.Refresh(ctx, b); err != nil {
				b.LiveNote = "balance refresh failed"
			}
			resub = true
			dirty = true
		case <-draw.C:
			if dirty {
				paint()
				dirty = false
			}
		}
	}
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > 30*time.Second {
		return 30 * time.Second
	}
	if d < time.Second {
		return time.Second
	}
	return d
}

func withJitter(d time.Duration) time.Duration {
	j := time.Duration(rand.Intn(250)) * time.Millisecond
	return d + j
}
