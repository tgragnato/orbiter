package signal

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/internal/broker"
)

func TestMessageConstructors(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	order := broker.NewMarketOrder(broker.BuyDirectionLong, 2.5, "VWCE.DE", decimal.Zero, decimal.Zero)
	order.ID = "o1"

	buy := NewBuyMessage(now, order)
	if buy.Type != TypeBuy {
		t.Fatalf("buy.Type = %q, want %q", buy.Type, TypeBuy)
	}
	if buy.Order == nil || buy.Order.ID != "o1" {
		t.Fatalf("buy.Order mismatch: %#v", buy.Order)
	}

	cancel := NewCancelOrderMessage(now, order)
	if cancel.Type != TypeCancelOrder {
		t.Fatalf("cancel.Type = %q, want %q", cancel.Type, TypeCancelOrder)
	}
	if cancel.OrderID != "o1" {
		t.Fatalf("cancel.OrderID = %q, want o1", cancel.OrderID)
	}

	position := broker.Position{Reference: "p1", Instrument: "SWDA.MI", BuyDirection: broker.BuyDirectionShort, Size: 1.25}
	sell := NewSellMessage(now, position)
	if sell.Type != TypeSell {
		t.Fatalf("sell.Type = %q, want %q", sell.Type, TypeSell)
	}
	if sell.Position == nil || sell.Position.Reference != "p1" {
		t.Fatalf("sell.Position mismatch: %#v", sell.Position)
	}
}

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
