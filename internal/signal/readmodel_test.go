package signal_test

import (
	"testing"
	"time"

	"github.com/tgragnato/orbiter/internal/signal"
)

func TestReadModelWithMemoryDispatcher(t *testing.T) {
	t.Parallel()

	dispatcher := signal.NewMemoryDispatcher()
	_ = dispatcher.Dispatch(signal.Message{
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
	})
	_ = dispatcher.Dispatch(signal.Message{
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
	})

	readModel := signal.NewReadModel(dispatcher)

	if got := readModel.Pending(); len(got) != 2 {
		t.Fatalf("Pending len = %d, want 2", len(got))
	}

	if got := readModel.Drain(); len(got) != 2 {
		t.Fatalf("Drain len = %d, want 2", len(got))
	}

	if got := readModel.Pending(); len(got) != 0 {
		t.Fatalf("Pending after drain = %d, want 0", len(got))
	}
}

func TestReadModelWithNonQueueDispatcher(t *testing.T) {
	t.Parallel()

	readModel := signal.NewReadModel(noopDispatcher{})

	if got := readModel.Pending(); got != nil {
		t.Fatalf("Pending = %#v, want nil", got)
	}

	if got := readModel.Drain(); got != nil {
		t.Fatalf("Drain = %#v, want nil", got)
	}
}

type noopDispatcher struct{}

func (n noopDispatcher) Dispatch(_ signal.Message) error {
	return nil
}
