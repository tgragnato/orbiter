package trader

import (
	"fmt"
	"log/slog"
	"sort"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/helper"
)

var dec100 = decimal.NewFromFloat(100)

// distanceInPercentage - distance between price1 and price2 in %
func distanceInPercentage(price1, price2 decimal.Decimal) decimal.Decimal {
	if price1.IsZero() {
		return decimal.Zero
	}
	return price2.Sub(price1).Div(price1).Mul(dec100)
}

func (tr *Trader) printPositionPerformanceByNotes() {
	closedPositions, err := tr.GetClosedPositions()
	if err != nil {
		slog.Error("cannot get closed positions for performance summary", "error", err)
		return
	}

	var perfPositionsByNote = map[string]float64{}
	for _, position := range closedPositions {
		perfInPips := helper.Cent2Pips(decimal.NewFromFloat(position.PerformanceAbsolute(decimal.Zero, decimal.Zero)))
		perfInPipsFloat, _ := perfInPips.Float64()
		key := fmt.Sprintf("%d-%s", position.BuyTime.Year(), position.Reference)
		perfPositionsByNote[key] += perfInPipsFloat
	}
	var sortedKeys []string
	for key := range perfPositionsByNote {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)
	for _, note := range sortedKeys {
		totalProfit := perfPositionsByNote[note]
		slog.Info("position performance by note", "note", note, "total_profit_pips", totalProfit)
	}
}
