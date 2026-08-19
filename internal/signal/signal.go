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
	Summary       string  // brief human-readable label, suitable for logs; not the primary display
	OrderID       string
	Conviction    float64 // ML conviction score that produced this signal; 0 for alerts
	TargetWeight  float64 // target allocation as fraction of total satellite NAV [0,1]
	CurrentWeight float64 // current allocation as fraction of total satellite NAV [0,1]
	Delta         float64 // monetary change in the portfolio base currency; positive = buy, negative = sell
	Currency      string  // ISO 4217 code of the portfolio base currency (e.g. "EUR")
	MarketPrice   float64 // for CORE_PMC_FLOOR_ALERT: current market price
	PMC           float64 // for CORE_PMC_FLOOR_ALERT: Prezzo Medio di Carico
}

// Dispatcher receives signals for downstream presentation/handling.
type Dispatcher interface {
	Dispatch(message Message) error
}

// NewRebalanceMessage builds a tactical rebalance signal for a Satellite holding.
func NewRebalanceMessage(
	now time.Time,
	symbol string,
	conviction float64,
	direction string,
	currentWeight, targetWeight, delta float64,
	currency string,
) Message {
	return Message{
		Type:          TypeRebalance,
		CreatedAt:     now,
		Instrument:    symbol,
		Summary:       fmt.Sprintf("Rebalance %s %s", symbol, direction),
		OrderID:       "",
		Conviction:    conviction,
		CurrentWeight: currentWeight,
		TargetWeight:  targetWeight,
		Delta:         delta,
		Currency:      currency,
		MarketPrice:   0,
		PMC:           0,
	}
}

// NewBuyMessage suggests opening a new satellite position for a symbol not currently held.
// targetWeight is the suggested allocation as a fraction of total satellite NAV; delta is
// the recommended investment amount in the portfolio base currency.
func NewBuyMessage(now time.Time, symbol string, conviction, targetWeight, delta float64, currency string) Message {
	direction := "long"
	if conviction < 0 {
		direction = "short"
	}

	return Message{
		Type:          TypeBuy,
		CreatedAt:     now,
		Instrument:    symbol,
		Summary:       fmt.Sprintf("Entry %s %s", symbol, direction),
		OrderID:       "",
		Conviction:    conviction,
		TargetWeight:  targetWeight,
		CurrentWeight: 0,
		Delta:         delta,
		Currency:      currency,
		MarketPrice:   0,
		PMC:           0,
	}
}

// NewSellMessage suggests closing an existing satellite position entirely.
// currentWeight is the holding's current allocation as a fraction of total satellite NAV;
// delta is the recovered amount in the portfolio base currency (negative: selling reduces exposure).
func NewSellMessage(now time.Time, symbol string, conviction, currentWeight, delta float64, currency string) Message {
	direction := "long"
	if conviction < 0 {
		direction = "short"
	}

	return Message{
		Type:          TypeSell,
		CreatedAt:     now,
		Instrument:    symbol,
		Summary:       fmt.Sprintf("Exit %s %s", symbol, direction),
		OrderID:       "",
		Conviction:    conviction,
		CurrentWeight: currentWeight,
		TargetWeight:  0,
		Delta:         delta,
		Currency:      currency,
		MarketPrice:   0,
		PMC:           0,
	}
}

// NewCorePMCFloorAlert builds an alert signal when a Core holding is at or below PMC.
func NewCorePMCFloorAlert(now time.Time, symbol string, marketPrice, pmc float64) Message {
	return Message{
		Type:          TypeCorePMCFloorAlert,
		CreatedAt:     now,
		Instrument:    symbol,
		Summary:       "PMC floor alert: " + symbol,
		OrderID:       "",
		Conviction:    0,
		TargetWeight:  0,
		CurrentWeight: 0,
		Delta:         0,
		Currency:      "",
		MarketPrice:   marketPrice,
		PMC:           pmc,
	}
}
