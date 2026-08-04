package broker

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestMergeOrders(t *testing.T) {
	t.Parallel()

	order1 := NewMarketOrder(BuyDirectionShort, 1.00, "", decimal.Zero, decimal.Zero)
	order2 := NewMarketOrder(BuyDirectionShort, 1.00, "", decimal.Zero, decimal.Zero)
	orders1 := []Order{order1}
	orders2 := []Order{order2}

	orders := MergeOrders(orders1, orders2)
	if len(orders) != 2 {
		t.Fatalf("expected %d, got %d", 2, len(orders))
	}
}
