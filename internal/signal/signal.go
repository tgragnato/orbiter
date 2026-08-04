package signal

import (
	"fmt"
	"time"

	"github.com/tgragnato/orbiter/pkg/broker"
)

// Type describes the signal intent.
type Type string

const (
	// TypeBuy requests opening a new position.
	TypeBuy Type = "BUY"
	// TypeSell requests closing an open position.
	TypeSell Type = "SELL"
	// TypeCancelOrder requests canceling an open order.
	TypeCancelOrder Type = "CANCEL_ORDER"
	// TypeRebalance requests applying a target allocation change.
	TypeRebalance Type = "REBALANCE"
	// TypeCorePMCFloorAlert fires when a Core holding price falls to or below PMC.
	TypeCorePMCFloorAlert Type = "CORE_PMC_FLOOR_ALERT"
	// TypeEntry suggests opening a new satellite position not currently held.
	TypeEntry Type = "ENTRY"
)

// Message is an execution intent emitted by trader/strategy and consumed by UI.
type Message struct {
	Type          Type
	CreatedAt     time.Time
	Instrument    string
	Summary       string
	OrderID       string
	Order         *broker.Order
	Position      *broker.Position
	TargetWeight  float64 // target allocation as fraction of total satellite NAV [0,1]
	CurrentWeight float64 // current allocation as fraction of total satellite NAV [0,1]
	DeltaEUR      float64 // positive = buy, negative = sell
}

// Dispatcher receives signals for downstream presentation/handling.
type Dispatcher interface {
	Dispatch(message Message) error
}

// NewBuyMessage builds a buy signal from an order.
func NewBuyMessage(now time.Time, order broker.Order) Message {
	copyOrder := order
	return Message{
		Type:       TypeBuy,
		CreatedAt:  now,
		Instrument: order.Instrument,
		Summary:    fmt.Sprintf("Buy %s %.2f", order.Direction, order.Size),
		Order:      &copyOrder,
	}
}

// NewSellMessage builds a sell signal from a position.
func NewSellMessage(now time.Time, position broker.Position) Message {
	copyPosition := position
	return Message{
		Type:       TypeSell,
		CreatedAt:  now,
		Instrument: position.Instrument,
		Summary:    fmt.Sprintf("Sell %s %.2f", position.BuyDirection, position.Size),
		Position:   &copyPosition,
	}
}

// NewCancelOrderMessage builds an order cancellation signal.
func NewCancelOrderMessage(now time.Time, order broker.Order) Message {
	copyOrder := order
	return Message{
		Type:       TypeCancelOrder,
		CreatedAt:  now,
		Instrument: order.Instrument,
		Summary:    fmt.Sprintf("Cancel order %s", order.ID),
		OrderID:    order.ID,
		Order:      &copyOrder,
	}
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

// NewEntryMessage suggests opening a new satellite position for a symbol not currently held.
// targetWeight is the suggested allocation as a fraction of total satellite NAV; deltaEUR is
// the recommended investment amount.
func NewEntryMessage(now time.Time, symbol string, conviction, targetWeight, deltaEUR float64) Message {
	direction := "long"
	if conviction < 0 {
		direction = "short"
	}
	return Message{
		Type:         TypeEntry,
		CreatedAt:    now,
		Instrument:   symbol,
		Summary:      fmt.Sprintf("Entry %s %s target %.1f%% (Δ%.0f€, conviction %.2f)", symbol, direction, targetWeight*100, deltaEUR, conviction),
		TargetWeight: targetWeight,
		DeltaEUR:     deltaEUR,
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
