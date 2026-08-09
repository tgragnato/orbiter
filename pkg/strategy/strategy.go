package strategy

import (
	"time"

	"github.com/tgragnato/orbiter/pkg/broker"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/tick"
)

const (
	NameDOJI       = "doji"
	NameHeikinAshi = "heikinashi"
	NameScalper    = "scalper"
	NameStochRSI   = "stochrsi"
	NameRSI        = "rsi"
	NameRSIADX     = "rsiadx"
	NameLowCandle  = "lowcandle"
	NameHarami     = "harami"
	NameSMA10      = "sma10"
	NameEngulfing  = "engulfing"
)

// Strategy is the core interface for candle-and-tick driven trading logic.
type Strategy interface {
	// Name returns the name of the strategy.
	Name() string

	// OnCandle processes the 100 most-recent closed candles.
	OnCandle(closedCandles []*ohlc.OHLC) (toOpen, toClose []broker.Order, toClosePositions []broker.Position)

	// OnTick processes each incoming price tick.
	OnTick(currentTick tick.Tick) (toOpen, toClose []broker.Order, toClosePositions []broker.Position)

	// OnPosition updates open and closed positions.
	OnPosition(openPositions []broker.Position, closedPositions []broker.Position)

	// OnOrder updates the currently open orders.
	OnOrder(openOrders []broker.Order)

	// OnWarmUpCandle feeds a historical candle to warm up indicators.
	OnWarmUpCandle(closedCandle *ohlc.OHLC)

	// GetWarmUpCandleAmount returns how many warm-up candles the strategy requests.
	GetWarmUpCandleAmount() uint

	// GetCandleDuration returns the candle period this strategy operates on.
	GetCandleDuration() time.Duration

	// String describes strategy parameters (stop-loss, target, etc.).
	String() string

	// Score returns a continuous conviction score used by the ML ensemble.
	// Score reads from the current indicator state (populated by the most recent OnCandle/OnWarmUpCandle call) without mutating it.
	// Return range: [-1.0, +1.0]
	//
	//	+1.0 = maximum buy/long conviction
	//	 0.0 = neutral / no signal
	//	-1.0 = maximum sell/short conviction
	Score(window []*ohlc.OHLC) float64
}
