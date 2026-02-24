package iso

import (
	"testing"
	"time"
)

func TestProgressCounter_NoTotal(t *testing.T) {
	pc := &progressCounter{
		total:     0,
		startTime: time.Now().Add(-time.Second),
	}
	if _, err := pc.Write([]byte("data")); err != nil {
		t.Fatalf("Write: %v", err)
	}
}
