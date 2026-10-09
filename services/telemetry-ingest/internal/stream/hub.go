// Package stream fans validated telemetry out to live subscribers (the
// dashboard's SSE connections) without letting a slow client stall ingest.
package stream

import (
	"sync"

	"github.com/jsingh9536/portifolio-t/services/telemetry-ingest/internal/telemetry"
)

type subscriber struct {
	site string // empty = all sites
	ch   chan telemetry.Sample
}

type Hub struct {
	mu     sync.RWMutex
	subs   map[*subscriber]struct{}
	OnDrop func() // called when a subscriber's buffer is full; for metrics
}

func NewHub() *Hub { return &Hub{subs: map[*subscriber]struct{}{}} }

// Subscribe returns a channel of samples for site ("" for every site) and a
// cancel func that must be called to release it.
func (h *Hub) Subscribe(site string, buffer int) (<-chan telemetry.Sample, func()) {
	s := &subscriber{site: site, ch: make(chan telemetry.Sample, buffer)}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return s.ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, s)
			h.mu.Unlock()
			close(s.ch)
		})
	}
}

// Publish never blocks: if a subscriber can't keep up, it misses samples.
// Dashboards only care about the newest state, so dropping is the right call.
func (h *Hub) Publish(samples []telemetry.Sample) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for s := range h.subs {
		for _, sample := range samples {
			if s.site != "" && s.site != sample.SiteID {
				continue
			}
			select {
			case s.ch <- sample:
			default:
				if h.OnDrop != nil {
					h.OnDrop()
				}
			}
		}
	}
}

func (h *Hub) Subscribers() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}
