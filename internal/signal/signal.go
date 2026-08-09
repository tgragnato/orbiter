package signal

import (
	"fmt"
	"time"
)

// Type describes the signal intent.
type Type string

const (
	// TypeBuy suggests opening a new satellite position not currently held.
	TypeBuy Type = "BUY"
	// TypeSell suggests closing an existing satellite position entirely.
	TypeSell Type = "SELL"
	// TypeRebalance requests applying a target allocation change.
	TypeRebalance Type = "REBALANCE"
	// TypeCorePMCFloorAlert fires when a Core holding price falls to or below PMC.
	TypeCorePMCFloorAlert Type = "CORE_PMC_FLOOR_ALERT"
)

// Message is an execution intent emitted by trader/strategy and consumed by UI.
type Message struct {
	Type          Type
	CreatedAt     time.Time
	Instrument    string
	Summary       string
	OrderID       string
	TargetWeight  float64 // target allocation as fraction of total satellite NAV [0,1]
	CurrentWeight float64 // current allocation as fraction of total satellite NAV [0,1]
	DeltaEUR      float64 // positive = buy, negative = sell
}

// Dispatcher receives signals for downstream presentation/handling.
type Dispatcher interface {
	Dispatch(message Message) error
}

// NewRebalanceMessage builds a tactical rebalance signal for a Satellite holding.
func NewRebalanceMessage(now time.Time, symbol string, conviction float64, direction string, currentWeight, targetWeight, deltaEUR float64) Message {
	return Message{
		Type:          TypeRebalance,
		CreatedAt:     now,
		Instrument:    symbol,
		Summary:       fmt.Sprintf("Rebalance %s %s %.1f%%→%.1f%% (Δ%.0f€, conviction %.2f)", symbol, direction, currentWeight*100, targetWeight*100, deltaEUR, conviction),
		CurrentWeight: currentWeight,
		TargetWeight:  targetWeight,
		DeltaEUR:      deltaEUR,
	}
}

// NewBuyMessage suggests opening a new satellite position for a symbol not currently held.
// targetWeight is the suggested allocation as a fraction of total satellite NAV; deltaEUR is
// the recommended investment amount.
func NewBuyMessage(now time.Time, symbol string, conviction, targetWeight, deltaEUR float64) Message {
	direction := "long"
	if conviction < 0 {
		direction = "short"
	}
	return Message{
		Type:         TypeBuy,
		CreatedAt:    now,
		Instrument:   symbol,
		Summary:      fmt.Sprintf("Entry %s %s target %.1f%% (Δ%.0f€, conviction %.2f)", symbol, direction, targetWeight*100, deltaEUR, conviction),
		TargetWeight: targetWeight,
		DeltaEUR:     deltaEUR,
	}
}

// NewSellMessage suggests closing an existing satellite position entirely.
// currentWeight is the holding's current allocation as a fraction of total satellite NAV;
// deltaEUR is the recovered amount (negative: selling reduces exposure).
func NewSellMessage(now time.Time, symbol string, conviction, currentWeight, deltaEUR float64) Message {
	direction := "long"
	if conviction < 0 {
		direction = "short"
	}
	return Message{
		Type:          TypeSell,
		CreatedAt:     now,
		Instrument:    symbol,
		Summary:       fmt.Sprintf("Exit %s %s from %.1f%% (Δ%.0f€, conviction %.2f)", symbol, direction, currentWeight*100, deltaEUR, conviction),
		CurrentWeight: currentWeight,
		TargetWeight:  0,
		DeltaEUR:      deltaEUR,
	}
}

// NewCorePMCFloorAlert builds an alert signal when a Core holding is at or below PMC.
func NewCorePMCFloorAlert(now time.Time, symbol string, marketPrice, pmc float64) Message {
	return Message{
		Type:       TypeCorePMCFloorAlert,
		CreatedAt:  now,
		Instrument: symbol,
		Summary:    fmt.Sprintf("CORE PMC FLOOR: %s market=%.4f pmc=%.4f", symbol, marketPrice, pmc),
	}
}
