package signal_test

import (
	"testing"
	"time"

	"github.com/tgragnato/orbiter/internal/signal"
)

//nolint:funlen // test function needs to cover the full MemoryDispatcher API surface.
func TestMemoryDispatcher(t *testing.T) {
	t.Parallel()

	dispatcher := signal.NewMemoryDispatcher()
	msg1 := signal.Message{
		Type:          signal.TypeBuy,
		Instrument:    "A",
		CreatedAt:     time.Time{},
		Summary:       "",
		OrderID:       "",
		Conviction:    0,
		TargetWeight:  0,
		CurrentWeight: 0,
		Delta:         0,
		Currency:      "",
		MarketPrice:   0,
		PMC:           0,
	}
	msg2 := signal.Message{
		Type:          signal.TypeSell,
		Instrument:    "B",
		CreatedAt:     time.Time{},
		Summary:       "",
		OrderID:       "",
		Conviction:    0,
		TargetWeight:  0,
		CurrentWeight: 0,
		Delta:         0,
		Currency:      "",
		MarketPrice:   0,
		PMC:           0,
	}

	err := dispatcher.Dispatch(msg1)
	if err != nil {
		t.Fatalf("Dispatch(msg1) error = %v", err)
	}

	err = dispatcher.Dispatch(msg2)
	if err != nil {
		t.Fatalf("Dispatch(msg2) error = %v", err)
	}

	msgs := dispatcher.Messages()
	if len(msgs) != 2 {
		t.Fatalf("Messages len = %d, want 2", len(msgs))
	}

	msgs[0].Instrument = "mutated"

	check := dispatcher.Messages()
	if check[0].Instrument != "A" {
		t.Fatalf("Messages should return copy, got %q", check[0].Instrument)
	}

	drained := dispatcher.Drain()
	if len(drained) != 2 {
		t.Fatalf("Drain len = %d, want 2", len(drained))
	}

	if len(dispatcher.Messages()) != 0 {
		t.Fatalf("expected queue to be empty after Drain")
	}
}
