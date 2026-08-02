package trader

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/sklinkert/at/internal/broker"
)

var lossPosition = broker.Position{
	BuyPrice:     decimal.NewFromFloat(2),
	SellPrice:    decimal.NewFromFloat(1),
	BuyDirection: broker.BuyDirectionLong,
}

var winPosition = broker.Position{
	BuyPrice:     decimal.NewFromFloat(1),
	SellPrice:    decimal.NewFromFloat(2),
	BuyDirection: broker.BuyDirectionLong,
}

func TestTrader_getMaxConsecutiveLossTrades(t *testing.T) {
	t.Parallel()

	var tr = Trader{}
	var closedPositions = []broker.Position{
		winPosition,
		lossPosition,
		lossPosition,
		winPosition,
		winPosition,
		lossPosition,
	}
	if 2 != int(tr.getMaxConsecutiveLossTrades(closedPositions)) {
		t.Fatalf("expected %d, got %d", 2, int(tr.getMaxConsecutiveLossTrades(closedPositions)))
	}
}
