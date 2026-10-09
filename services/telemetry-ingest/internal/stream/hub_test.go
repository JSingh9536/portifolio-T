package stream

import (
	"testing"

	"github.com/jsingh9536/portifolio-t/services/telemetry-ingest/internal/telemetry"
)

func TestHubFiltersBySiteAndDropsWhenFull(t *testing.T) {
	h := NewHub()
	drops := 0
	h.OnDrop = func() { drops++ }

	siteA, cancelA := h.Subscribe("a", 1)
	all, cancelAll := h.Subscribe("", 10)
	defer cancelAll()

	h.Publish([]telemetry.Sample{{RobotID: "1", SiteID: "a"}, {RobotID: "2", SiteID: "b"}, {RobotID: "3", SiteID: "a"}})

	if got := (<-siteA).RobotID; got != "1" {
		t.Fatalf("site a got %s", got)
	}
	if drops != 1 {
		t.Fatalf("expected 1 drop for the full site-a buffer, got %d", drops)
	}
	if len(all) != 3 {
		t.Fatalf("all-sites subscriber should get 3, got %d", len(all))
	}

	cancelA()
	cancelA() // idempotent
	if h.Subscribers() != 1 {
		t.Fatalf("subscribers = %d", h.Subscribers())
	}
	if _, ok := <-siteA; ok {
		t.Fatal("cancelled channel should be closed")
	}
}
