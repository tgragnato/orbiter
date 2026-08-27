// Copyright (c) 2019 Simon Klinkert
// Copyright (c) 2026 Tommaso Gragnato
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package strategy

import (
	"time"

	"github.com/tgragnato/orbiter/pkg/broker"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// Strategy name constants used to identify each trading strategy.
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
	// Score reads from the current indicator state (populated by the most recent OnCandle/OnWarmUpCandle call)
	// without mutating it.
	// Return range: [-1.0, +1.0]
	//
	//	+1.0 = maximum buy/long conviction
	//	 0.0 = neutral / no signal
	//	-1.0 = maximum sell/short conviction
	Score(window []*ohlc.OHLC) float64
}
