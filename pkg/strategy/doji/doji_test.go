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

//nolint:testpackage // accesses unexported fields (previousCandle)
package doji

import (
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/broker"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

func TestDoji_Name(t *testing.T) {
	t.Parallel()

	dojiStrat := New("test")
	if strategy.NameDOJI != dojiStrat.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameDOJI, dojiStrat.Name())
	}
}

func TestDoji_OnWarmUpCandle(t *testing.T) {
	t.Parallel()

	dojiStrat := New("test")
	candle := ohlc.New("test", time.Now(), time.Minute*60, false)
	candle.NewPrice(100, candle.Start)
	candle.ForceClose()

	dojiStrat.OnWarmUpCandle(candle)

	if dojiStrat.previousCandle != candle {
		t.Errorf("expected previousCandle to be updated by OnWarmUpCandle")
	}
}

func TestDoji_OnTick_Long(t *testing.T) {
	t.Parallel()

	dojiStrat := New("test")
	// Mock a DOJI candle.
	prevCandle := ohlc.New("test", time.Now(), time.Minute*60, false)
	prevCandle.NewPrice(100, prevCandle.Start)
	prevCandle.NewPrice(100.01, prevCandle.Start)
	prevCandle.ForceClose()
	// High: 100.01, Low: 100

	dojiStrat.OnCandle([]*ohlc.OHLC{prevCandle})

	// Tick breaks high by > 2 pips (0.0002).
	// 100.01 + 0.0002 = 100.0102
	currentTick := tick.New("test", time.Now(), 100.0103, 100.0103)

	toOpen, _, _ := dojiStrat.OnTick(currentTick)
	if len(toOpen) != 1 {
		t.Fatalf("expected 1 order, got %d", len(toOpen))
	}

	if toOpen[0].Direction != broker.BuyDirectionLong {
		t.Fatalf("expected BuyDirectionLong, got %v", toOpen[0].Direction)
	}
}

func TestDoji_OnTick_Short(t *testing.T) {
	t.Parallel()

	dojiStrat := New("test")
	// Mock a DOJI candle.
	prevCandle := ohlc.New("test", time.Now(), time.Minute*60, false)
	prevCandle.NewPrice(100, prevCandle.Start)
	prevCandle.NewPrice(100.01, prevCandle.Start)
	prevCandle.ForceClose()
	// High: 100.01, Low: 100

	dojiStrat.OnCandle([]*ohlc.OHLC{prevCandle})

	// Tick breaks low by > 2 pips (0.0002).
	// 100 - 0.0002 = 99.9998
	currentTick := tick.New("test", time.Now(), 99.9997, 99.9997)

	toOpen, _, _ := dojiStrat.OnTick(currentTick)
	if len(toOpen) != 1 {
		t.Fatalf("expected 1 order, got %d", len(toOpen))
	}

	if toOpen[0].Direction != broker.BuyDirectionShort {
		t.Fatalf("expected BuyDirectionShort, got %v", toOpen[0].Direction)
	}
}

//nolint:funlen // test function covers multiple subtests
func TestDoji_Score(t *testing.T) {
	t.Parallel()

	t.Run("Nil Previous Candle", func(t *testing.T) {
		t.Parallel()

		dojiStrat := New("test")

		got := dojiStrat.Score(nil)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when previousCandle is nil", got)
		}
	})

	t.Run("DOJI Candle Active", func(t *testing.T) {
		t.Parallel()

		dojiStrat := New("test")
		candle := ohlc.New("test", time.Now(), time.Minute*60, false)
		candle.NewPrice(100, candle.Start)
		candle.NewPrice(100.01, candle.Start)
		candle.ForceClose()

		dojiStrat.OnWarmUpCandle(candle)

		got := dojiStrat.Score(nil)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when previousCandle is DOJI", got)
		}
	})

	t.Run("Non-DOJI Normal Range", func(t *testing.T) {
		t.Parallel()

		dojiStrat := New("test")
		candle := ohlc.New("test", time.Now(), time.Minute*60, false)
		candle.NewPrice(100, candle.Start)
		candle.NewPrice(105, candle.Start) // 5% performance, not DOJI
		candle.ForceClose()

		dojiStrat.OnWarmUpCandle(candle)

		got := dojiStrat.Score(nil)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when close is within range", got)
		}
	})

	t.Run("Close Above High Breakout", func(t *testing.T) {
		t.Parallel()

		dojiStrat := New("test")
		candle := &ohlc.OHLC{
			Instrument: "",
			Open:       100.0,
			High:       102.0,
			HighTime:   time.Time{},
			Low:        99.0,
			LowTime:    time.Time{},
			Close:      103.0, // Close > High
			Start:      time.Now(),
			End:        time.Now().Add(time.Hour),
			Duration:   0,
			Gaps:       false,
		}
		candle.ForceClose()

		dojiStrat.OnWarmUpCandle(candle)

		got := dojiStrat.Score(nil)
		if got <= 0.0 {
			t.Errorf("Score() = %v, want > 0.0 when Close > High", got)
		}
	})

	t.Run("Close Below Low Breakout", func(t *testing.T) {
		t.Parallel()

		dojiStrat := New("test")
		candle := &ohlc.OHLC{
			Instrument: "",
			Open:       100.0,
			High:       101.0,
			HighTime:   time.Time{},
			Low:        98.0,
			LowTime:    time.Time{},
			Close:      97.0, // Close < Low
			Start:      time.Now(),
			End:        time.Now().Add(time.Hour),
			Duration:   0,
			Gaps:       false,
		}
		candle.ForceClose()

		dojiStrat.OnWarmUpCandle(candle)

		got := dojiStrat.Score(nil)
		if got >= 0.0 {
			t.Errorf("Score() = %v, want < 0.0 when Close < Low", got)
		}
	})
}
