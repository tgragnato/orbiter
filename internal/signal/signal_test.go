package signal

import (
	"testing"
)

func TestMemoryDispatcher(t *testing.T) {
	t.Parallel()

	d := NewMemoryDispatcher()
	m1 := Message{Type: TypeBuy, Instrument: "A"}
	m2 := Message{Type: TypeSell, Instrument: "B"}

	if err := d.Dispatch(m1); err != nil {
		t.Fatalf("Dispatch(m1) error = %v", err)
	}
	if err := d.Dispatch(m2); err != nil {
		t.Fatalf("Dispatch(m2) error = %v", err)
	}

	msgs := d.Messages()
	if len(msgs) != 2 {
		t.Fatalf("Messages len = %d, want 2", len(msgs))
	}

	msgs[0].Instrument = "mutated"
	check := d.Messages()
	if check[0].Instrument != "A" {
		t.Fatalf("Messages should return copy, got %q", check[0].Instrument)
	}

	drained := d.Drain()
	if len(drained) != 2 {
		t.Fatalf("Drain len = %d, want 2", len(drained))
	}
	if len(d.Messages()) != 0 {
		t.Fatalf("expected queue to be empty after Drain")
	}
}
