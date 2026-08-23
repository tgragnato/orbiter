package sma10_test

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"

	"github.com/tgragnato/orbiter/pkg/strategy/sma10"
)

const testInstrument = "CAD/AUD"

func TestSMA10_Name(t *testing.T) {
	t.Parallel()

	s := sma10.New("test", time.Minute*60)
	if strategy.NameSMA10 != s.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameSMA10, s.Name())
	}
}

func TestSMA10_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	s := sma10.New("test", time.Minute*60)
	currentTick := tick.New("test", time.Now(), 100, 100)

	toOpen, _, _ := s.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}

//nolint:cyclop,funlen // test function with multiple assertion branches and table entries
func TestSMA_Score(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		instrument     string
		candleDuration time.Duration
		closedCandles  []*ohlc.OHLC
		wantZero       bool
		wantPositive   bool
	}{
		{
			name:           "Less than 10 closed candles",
			instrument:     testInstrument,
			candleDuration: time.Hour,
			closedCandles:  generateCandles("uptrend", 2),
			wantZero:       true,
			wantPositive:   false,
		},
		{
			name:           "Test SMA Score Calculation (Mixed Price Action)",
			instrument:     testInstrument,
			candleDuration: time.Hour,
			closedCandles:  generateCandles("mixed", 99),
			wantZero:       true,
			wantPositive:   false,
		},
		{
			name:           "Test SMA Score Calculation (Long Uptrend)",
			instrument:     testInstrument,
			candleDuration: time.Hour,
			closedCandles:  generateCandles("uptrend", 1500),
			wantZero:       false,
			wantPositive:   true,
		},
		{
			name:           "Test SMA Score Calculation (Long Downtrend)",
			instrument:     testInstrument,
			candleDuration: time.Hour,
			closedCandles:  generateCandles("downtrend", 1500),
			wantZero:       false,
			wantPositive:   false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			d := sma10.New(testCase.instrument, testCase.candleDuration)
			for _, candle := range testCase.closedCandles {
				d.OnWarmUpCandle(candle)
			}

			got := d.Score(testCase.closedCandles)
			if testCase.wantZero && got != 0 {
				t.Errorf("Score() = %v, want zero", got)
			}

			if !testCase.wantZero && testCase.wantPositive && got < 0 {
				t.Errorf("Score() = %v, want positive", got)
			}

			if !testCase.wantZero && !testCase.wantPositive && got > 0 {
				t.Errorf("Score() = %v, want negative", got)
			}
		})
	}
}

// generateCandles creates a slice of OHLC candles with a specified trend.
// trend: "mixed", "uptrend", or "downtrend".
// count: the number of candles to generate.
const testCandleDuration = time.Hour

//nolint:funlen // inherently long due to multiple trend branches and candle construction
func generateCandles(trend string, count int) []*ohlc.OHLC {
	candles := make([]*ohlc.OHLC, count)
	currentPrice := rand.Float64() * 100
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for idx := range count - 1 {
		var openPrice, highPrice, lowPrice, closePrice float64

		switch trend {
		case "uptrend":
			openPrice = currentPrice
			highPrice = currentPrice + (float64(idx) * 1.0)
			lowPrice = currentPrice - 0.5
			closePrice = currentPrice + (float64(idx) * 1.5)
			currentPrice = closePrice
		case "downtrend":
			openPrice = currentPrice
			highPrice = currentPrice + 0.5
			lowPrice = currentPrice - (float64(idx) * 1.0)
			closePrice = currentPrice - (float64(idx) * 1.5)
			currentPrice = closePrice
		default:
			openPrice = currentPrice
			change := (float64(idx%3) - 1) * 1.0
			highPrice = currentPrice + 2.0
			lowPrice = currentPrice - 2.0
			closePrice = currentPrice + change
			currentPrice = closePrice
		}

		candle := ohlc.New(testInstrument, now.Add(time.Duration(idx)*testCandleDuration), testCandleDuration, false)
		candle.Open = openPrice
		candle.High = highPrice
		candle.Low = lowPrice
		candle.Close = closePrice
		candle.ForceClose()
		candles[idx] = candle
	}

	multiple := 1.0

	switch trend {
	case "uptrend", "downtrend":
		multiple = 0.90
	}

	lastCandle := ohlc.New("test", now.Add(time.Duration(len(candles)-1)*testCandleDuration), testCandleDuration, false)
	lastCandle.Open = candles[len(candles)-2].Open * multiple
	lastCandle.High = candles[len(candles)-2].High * multiple
	lastCandle.Low = candles[len(candles)-2].Low * multiple
	lastCandle.Close = candles[len(candles)-2].Close * multiple
	lastCandle.ForceClose()
	candles[len(candles)-1] = lastCandle

	return candles
}
