package worker

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAllowRestart_RateLimits(t *testing.T) {
	w := &Worker{
		restarts: map[uuid.UUID][]time.Time{},
		cfg:      Config{MaxRestarts: 3, RestartWindow: time.Minute},
	}
	id := uuid.New()
	for i := 0; i < 3; i++ {
		if !w.allowRestart(id) {
			t.Fatalf("attempt %d within limit should be allowed", i+1)
		}
	}
	if w.allowRestart(id) {
		t.Fatal("4th attempt should be denied (limit exceeded)")
	}
}

func TestAllowRestart_WindowExpiry(t *testing.T) {
	w := &Worker{
		restarts: map[uuid.UUID][]time.Time{},
		cfg:      Config{MaxRestarts: 1, RestartWindow: 50 * time.Millisecond},
	}
	id := uuid.New()
	if !w.allowRestart(id) {
		t.Fatal("first attempt should be allowed")
	}
	if w.allowRestart(id) {
		t.Fatal("second attempt within window should be denied")
	}
	time.Sleep(70 * time.Millisecond)
	if !w.allowRestart(id) {
		t.Fatal("attempt after window expiry should be allowed again")
	}
}

func TestBeginEndHealing(t *testing.T) {
	w := &Worker{healing: map[uuid.UUID]bool{}}
	id := uuid.New()
	if !w.beginHealing(id) {
		t.Fatal("first beginHealing should succeed")
	}
	if w.beginHealing(id) {
		t.Fatal("second beginHealing should fail while in flight")
	}
	w.endHealing(id)
	if !w.beginHealing(id) {
		t.Fatal("beginHealing should succeed after endHealing")
	}
}
