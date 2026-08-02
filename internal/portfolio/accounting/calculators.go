package accounting

import (
	"errors"
	"fmt"
	"sort"
)

const quantityTolerance = 1e-9

var errInsufficientQuantity = errors.New("insufficient quantity in tax lots")

// PMCCalculator implements weighted average cost realization.
type PMCCalculator struct{}

// Calculate realizes PnL by applying one pooled weighted average cost.
func (PMCCalculator) Calculate(lots []TaxLot, sell SellTransaction) (RealizedPnLResult, []TaxLot, error) {
	if sell.Quantity <= 0 {
		return RealizedPnLResult{}, nil, fmt.Errorf("sell quantity must be > 0")
	}

	working := cloneLots(lots)
	totalQty := 0.0
	totalCost := 0.0
	for _, lot := range working {
		if lot.QuantityRemaining <= 0 {
			continue
		}
		totalQty += lot.QuantityRemaining
		totalCost += lot.QuantityRemaining * lot.UnitCost
	}
	if totalQty+quantityTolerance < sell.Quantity {
		return RealizedPnLResult{}, nil, errInsufficientQuantity
	}

	avgCost := totalCost / totalQty
	remaining := sell.Quantity
	consumptions := make([]LotConsumption, 0, len(working))

	for i := range working {
		if working[i].QuantityRemaining <= 0 {
			continue
		}

		consume := sell.Quantity * (working[i].QuantityRemaining / totalQty)
		if i == len(working)-1 || consume > remaining {
			consume = remaining
		}
		if consume <= 0 {
			continue
		}

		working[i].QuantityRemaining -= consume
		if working[i].QuantityRemaining < quantityTolerance {
			working[i].QuantityRemaining = 0
		}

		consumptions = append(consumptions, LotConsumption{
			TaxLotID:    working[i].ID,
			SourceLotID: working[i].ID,
			AcquiredAt:  working[i].AcquiredAt,
			Quantity:    consume,
			UnitCost:    avgCost,
			CostAmount:  consume * avgCost,
		})
		remaining -= consume
		if remaining <= quantityTolerance {
			break
		}
	}

	result := buildResult(sell, consumptions)
	return result, working, nil
}

// FIFOCalculator implements first-in-first-out lot realization.
type FIFOCalculator struct{}

// Calculate realizes PnL by consuming oldest lots first.
func (FIFOCalculator) Calculate(lots []TaxLot, sell SellTransaction) (RealizedPnLResult, []TaxLot, error) {
	return calculateByOrder(lots, sell, true)
}

// LIFOCalculator implements last-in-first-out lot realization.
type LIFOCalculator struct{}

// Calculate realizes PnL by consuming newest lots first.
func (LIFOCalculator) Calculate(lots []TaxLot, sell SellTransaction) (RealizedPnLResult, []TaxLot, error) {
	return calculateByOrder(lots, sell, false)
}

func calculateByOrder(lots []TaxLot, sell SellTransaction, ascending bool) (RealizedPnLResult, []TaxLot, error) {
	if sell.Quantity <= 0 {
		return RealizedPnLResult{}, nil, fmt.Errorf("sell quantity must be > 0")
	}

	working := cloneLots(lots)
	sort.SliceStable(working, func(i, j int) bool {
		if ascending {
			return working[i].AcquiredAt.Before(working[j].AcquiredAt)
		}
		return working[i].AcquiredAt.After(working[j].AcquiredAt)
	})

	remaining := sell.Quantity
	consumptions := make([]LotConsumption, 0)
	for i := range working {
		if remaining <= quantityTolerance {
			break
		}
		if working[i].QuantityRemaining <= 0 {
			continue
		}

		consume := min(working[i].QuantityRemaining, remaining)
		working[i].QuantityRemaining -= consume
		if working[i].QuantityRemaining < quantityTolerance {
			working[i].QuantityRemaining = 0
		}
		consumptions = append(consumptions, LotConsumption{
			TaxLotID:    working[i].ID,
			SourceLotID: working[i].ID,
			AcquiredAt:  working[i].AcquiredAt,
			Quantity:    consume,
			UnitCost:    working[i].UnitCost,
			CostAmount:  consume * working[i].UnitCost,
		})
		remaining -= consume
	}

	if remaining > quantityTolerance {
		return RealizedPnLResult{}, nil, errInsufficientQuantity
	}

	result := buildResult(sell, consumptions)
	return result, working, nil
}

func buildResult(sell SellTransaction, consumptions []LotConsumption) RealizedPnLResult {
	totalCost := 0.0
	for _, c := range consumptions {
		totalCost += c.CostAmount
	}
	proceeds := sell.Quantity * sell.UnitPrice
	return RealizedPnLResult{
		TotalProceeds: proceeds,
		TotalCost:     totalCost,
		RealizedPnL:   proceeds - totalCost,
		Consumptions:  consumptions,
	}
}

func cloneLots(lots []TaxLot) []TaxLot {
	copyLots := make([]TaxLot, len(lots))
	copy(copyLots, lots)
	return copyLots
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
