package mon

import (
	"sync"
	"testing"
	"time"
)

func TestCollectNowConcurrent(t *testing.T) {
	c := NewCollector(time.Hour, time.Hour) // tickers never fire
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.CollectNow()
		}()
	}
	wg.Wait()
	if c.Latest() == nil {
		t.Fatal("no snapshot after CollectNow")
	}
	if got := len(c.history); got != 8 {
		t.Fatalf("history = %d, want 8", got)
	}
}

func TestHistoryBound(t *testing.T) {
	c := NewCollector(time.Hour, time.Hour)
	for i := 0; i < c.maxHist+20; i++ {
		c.CollectNow()
	}
	if got := len(c.history); got != c.maxHist {
		t.Fatalf("history = %d, want capped %d", got, c.maxHist)
	}
	if got := len(c.History(c.maxHist + 50)); got != c.maxHist {
		t.Fatalf("History() = %d, want %d", got, c.maxHist)
	}
}

func TestStopIdempotent(t *testing.T) {
	c := NewCollector(time.Hour, time.Hour)
	done := make(chan struct{})
	go func() { c.Run(); close(done) }()
	c.Stop()
	c.Stop() // second call must not panic
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Stop")
	}
}
