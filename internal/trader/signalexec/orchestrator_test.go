package signalexec

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/internal/broker"
	"github.com/tgragnato/orbiter/internal/signal"
)

func TestDispatchOpenOrders(t *testing.T) {
	t.Parallel()

	dispatcher := signal.NewMemoryDispatcher()
	orchestrator := New(dispatcher, slog.Default())
	orchestrator.now = func() time.Time {
		return time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	}

	order := broker.NewMarketOrder(broker.BuyDirectionLong, 2.0, "VWCE.DE", decimal.Zero, decimal.Zero)
	var dispatchedOrders []broker.Order
	orchestrator.DispatchOpenOrders([]broker.Order{order}, "EUR", func(order broker.Order) {
		dispatchedOrders = append(dispatchedOrders, order)
	})

	if len(dispatchedOrders) != 1 {
		t.Fatalf("onDispatched calls = %d, want 1", len(dispatchedOrders))
	}
	if dispatchedOrders[0].CurrencyCode != "EUR" {
		t.Fatalf("currency code = %q, want EUR", dispatchedOrders[0].CurrencyCode)
	}

	msgs := dispatcher.Messages()
	if len(msgs) != 1 {
		t.Fatalf("messages len = %d, want 1", len(msgs))
	}
	if msgs[0].Type != signal.TypeBuy {
		t.Fatalf("type = %q, want %q", msgs[0].Type, signal.TypeBuy)
	}
}

func TestDispatchClosableOrdersAndPositions(t *testing.T) {
	t.Parallel()

	dispatcher := signal.NewMemoryDispatcher()
	orchestrator := New(dispatcher, slog.Default())
	orchestrator.now = func() time.Time { return time.Unix(123, 0).UTC() }

	order := broker.NewLimitOrder(broker.BuyDirectionShort, 1, "EUNL.DE", decimal.Zero, decimal.Zero, decimal.NewFromFloat(10))
	order.ID = "ord-1"
	position := broker.Position{Reference: "p1", Instrument: "SWDA.MI", BuyDirection: broker.BuyDirectionLong, Size: 1.1}

	orchestrator.DispatchClosableOrders([]broker.Order{order})
	orchestrator.DispatchClosablePositions([]broker.Position{position})

	msgs := dispatcher.Messages()
	if len(msgs) != 2 {
		t.Fatalf("messages len = %d, want 2", len(msgs))
	}
	if msgs[0].Type != signal.TypeCancelOrder {
		t.Fatalf("first type = %q, want %q", msgs[0].Type, signal.TypeCancelOrder)
	}
	if msgs[1].Type != signal.TypeSell {
		t.Fatalf("second type = %q, want %q", msgs[1].Type, signal.TypeSell)
	}
}

func TestPendingAndDrainMessages(t *testing.T) {
	t.Parallel()

	dispatcher := signal.NewMemoryDispatcher()
	readModel := signal.NewReadModel(dispatcher)

	_ = dispatcher.Dispatch(signal.Message{Type: signal.TypeBuy, Instrument: "A"})
	_ = dispatcher.Dispatch(signal.Message{Type: signal.TypeSell, Instrument: "B"})

	if len(readModel.Pending()) != 2 {
		t.Fatalf("Pending len = %d, want 2", len(readModel.Pending()))
	}
	if len(readModel.Drain()) != 2 {
		t.Fatalf("Drain len = %d, want 2", len(readModel.Drain()))
	}
	if len(readModel.Pending()) != 0 {
		t.Fatalf("expected empty queue after drain")
	}
}

func TestNonMemoryDispatcherAccessorsReturnNil(t *testing.T) {
	t.Parallel()

	readModel := signal.NewReadModel(noopDispatcher{})
	if got := readModel.Pending(); got != nil {
		t.Fatalf("Pending = %#v, want nil", got)
	}
	if got := readModel.Drain(); got != nil {
		t.Fatalf("Drain = %#v, want nil", got)
	}
}

func TestDispatchErrorsSkipCallback(t *testing.T) {
	t.Parallel()

	orchestrator := New(errDispatcher{}, slog.Default())

	called := false
	order := broker.NewMarketOrder(broker.BuyDirectionLong, 1.0, "A", decimal.Zero, decimal.Zero)
	orchestrator.DispatchOpenOrders([]broker.Order{order}, "USD", func(order broker.Order) {
		called = true
	})
	if called {
		t.Fatalf("callback should not be called when dispatch fails")
	}

	position := broker.Position{Reference: "p", Instrument: "A", BuyDirection: broker.BuyDirectionLong, Size: 1}
	orchestrator.DispatchClosablePositions([]broker.Position{position})
	orchestrator.DispatchClosableOrders([]broker.Order{{ID: "o", Instrument: "A"}})
}

type noopDispatcher struct{}

func (n noopDispatcher) Dispatch(message signal.Message) error {
	return nil
}

type errDispatcher struct{}

func (e errDispatcher) Dispatch(message signal.Message) error {
	return errors.New("dispatch failed")
}
