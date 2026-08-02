package signal

import "testing"

func TestReadModelWithMemoryDispatcher(t *testing.T) {
	t.Parallel()

	d := NewMemoryDispatcher()
	_ = d.Dispatch(Message{Type: TypeBuy, Instrument: "A"})
	_ = d.Dispatch(Message{Type: TypeSell, Instrument: "B"})

	rm := NewReadModel(d)
	if got := rm.Pending(); len(got) != 2 {
		t.Fatalf("Pending len = %d, want 2", len(got))
	}
	if got := rm.Drain(); len(got) != 2 {
		t.Fatalf("Drain len = %d, want 2", len(got))
	}
	if got := rm.Pending(); len(got) != 0 {
		t.Fatalf("Pending after drain = %d, want 0", len(got))
	}
}

func TestReadModelWithNonQueueDispatcher(t *testing.T) {
	t.Parallel()

	rm := NewReadModel(noopDispatcher{})
	if got := rm.Pending(); got != nil {
		t.Fatalf("Pending = %#v, want nil", got)
	}
	if got := rm.Drain(); got != nil {
		t.Fatalf("Drain = %#v, want nil", got)
	}
}

type noopDispatcher struct{}

func (n noopDispatcher) Dispatch(message Message) error {
	return nil
}
