package accounting

import "time"

// TaxLot represents one open buy lot tracked for cost basis and realization.
type TaxLot struct {
	ID                int64
	Symbol            string
	AcquiredAt        time.Time
	QuantityInitial   float64
	QuantityRemaining float64
	UnitCost          float64
}

// SellTransaction describes a sell fill requiring realized PnL attribution.
type SellTransaction struct {
	Symbol    string
	SoldAt    time.Time
	Quantity  float64
	UnitPrice float64
}

// LotConsumption links sold quantity to one specific tax lot.
type LotConsumption struct {
	TaxLotID    int64
	Quantity    float64
	UnitCost    float64
	CostAmount  float64
	AcquiredAt  time.Time
	SourceLotID int64
}

// RealizedPnLResult is the computed result for one sell transaction.
type RealizedPnLResult struct {
	TotalProceeds float64
	TotalCost     float64
	RealizedPnL   float64
	Consumptions  []LotConsumption
}

// CostBasisCalculator attributes sold quantity to lots and computes realized PnL.
type CostBasisCalculator interface {
	Calculate(lots []TaxLot, sell SellTransaction) (RealizedPnLResult, []TaxLot, error)
}
