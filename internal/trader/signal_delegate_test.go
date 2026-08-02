package trader

import (
	"log/slog"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/internal/broker"
	"github.com/tgragnato/orbiter/internal/signal"
)

func TestTraderSignalDelegation(t *testing.T) {
	t.Parallel()

	dispatcher := signal.NewMemoryDispatcher()
	tr := &Trader{
		clog:             slog.Default(),
		currencyCode:     "EUR",
		signalDispatcher: dispatcher,
	}

	order := broker.NewMarketOrder(broker.BuyDirectionLong, 1.0, "VWCE.DE", decimal.Zero, decimal.Zero)
	closeOrder := broker.NewLimitOrder(broker.BuyDirectionShort, 1.0, "VWCE.DE", decimal.Zero, decimal.Zero, decimal.NewFromFloat(10))
	closeOrder.ID = "ord-1"
	position := broker.Position{Reference: "pos-1", Instrument: "VWCE.DE", BuyDirection: broker.BuyDirectionLong, Size: 1.0}

	tr.processOrders([]broker.Order{order})
	tr.processClosableOrders([]broker.Order{closeOrder})
	tr.processClosablePositions([]broker.Position{position})

	readModel := signal.NewReadModel(dispatcher)
	msgs := readModel.Pending()
	if len(msgs) != 3 {
		t.Fatalf("Pending len = %d, want 3", len(msgs))
	}
	if msgs[0].Type != signal.TypeBuy || msgs[1].Type != signal.TypeCancelOrder || msgs[2].Type != signal.TypeSell {
		t.Fatalf("unexpected signal order/types: %#v", msgs)
	}
}

func TestSignalReadModelWithNonMemoryDispatcher(t *testing.T) {
	t.Parallel()

	readModel := signal.NewReadModel(testNoopDispatcher{})
	if got := readModel.Pending(); got != nil {
		t.Fatalf("Pending = %#v, want nil", got)
	}
	if got := readModel.Drain(); got != nil {
		t.Fatalf("Drain = %#v, want nil", got)
	}
}

type testNoopDispatcher struct{}

func (d testNoopDispatcher) Dispatch(message signal.Message) error {
	return nil
}
