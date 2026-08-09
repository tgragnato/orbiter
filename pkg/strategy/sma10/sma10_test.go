package sma10

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

func TestSMA10_Name(t *testing.T) {
	t.Parallel()

	s := New("test", time.Minute*60)
	if strategy.NameSMA10 != s.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameSMA10, s.Name())
	}
}

func TestSMA10_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	s := New("test", time.Minute*60)
	currentTick := tick.New("test", time.Now(), decimal.NewFromFloat(100), decimal.NewFromFloat(100))

	toOpen, _, _ := s.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}

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
			instrument:     "CAD/AUD",
			candleDuration: time.Hour,
			closedCandles:  generateCandles("uptrend", "CAD/AUD", 2, time.Hour),
			wantZero:       true,
			wantPositive:   false,
		},
		{
			name:           "Test SMA Score Calculation (Mixed Price Action)",
			instrument:     "CAD/AUD",
			candleDuration: time.Hour,
			closedCandles:  generateCandles("mixed", "CAD/AUD", 99, time.Hour),
			wantZero:       true,
			wantPositive:   false,
		},
		{
			name:           "Test SMA Score Calculation (Long Uptrend)",
			instrument:     "CAD/AUD",
			candleDuration: time.Hour,
			closedCandles:  generateCandles("uptrend", "CAD/AUD", 1500, time.Hour),
			wantZero:       false,
			wantPositive:   true,
		},
		{
			name:           "Test SMA Score Calculation (Long Downtrend)",
			instrument:     "CAD/AUD",
			candleDuration: time.Hour,
			closedCandles:  generateCandles("downtrend", "CAD/AUD", 1500, time.Hour),
			wantZero:       false,
			wantPositive:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := New(tt.instrument, tt.candleDuration)
			for _, candle := range tt.closedCandles {
				d.OnWarmUpCandle(candle)
			}

			got := d.Score(tt.closedCandles)
			if tt.wantZero && got != 0 {
				t.Errorf("Score() = %v, want zero", got)
			}
			if !tt.wantZero && tt.wantPositive && got < 0 {
				t.Errorf("Score() = %v, want positive", got)
			}
			if !tt.wantZero && !tt.wantPositive && got > 0 {
				t.Errorf("Score() = %v, want negative", got)
			}
		})
	}
}

// generateCandles creates a slice of OHLC candles with a specified trend.
// trend: "mixed", "uptrend", or "downtrend".
// count: the number of candles to generate.
// basePrice: the starting price for the candles.
func generateCandles(trend string, instrument string, count int, candleDuration time.Duration) []*ohlc.OHLC {
	candles := make([]*ohlc.OHLC, count)
	currentPrice := rand.Float64() * 100
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < count-1; i++ {
		var open, high, low, close float64

		switch trend {
		case "uptrend":
			open = currentPrice
			high = currentPrice + (float64(i) * 1.0)
			low = currentPrice - 0.5
			close = currentPrice + (float64(i) * 1.5)
			currentPrice = close
		case "downtrend":
			open = currentPrice
			high = currentPrice + 0.5
			low = currentPrice - (float64(i) * 1.0)
			close = currentPrice - (float64(i) * 1.5)
			currentPrice = close
		default:
			open = currentPrice
			change := (float64(i%3) - 1) * 1.0
			high = currentPrice + 2.0
			low = currentPrice - 2.0
			close = currentPrice + change
			currentPrice = close
		}

		o := ohlc.New(instrument, now.Add(time.Duration(i)*candleDuration), candleDuration, false)
		o.Open = decimal.NewFromFloat(open)
		o.High = decimal.NewFromFloat(high)
		o.Low = decimal.NewFromFloat(low)
		o.Close = decimal.NewFromFloat(close)
		o.ForceClose()
		candles[i] = o
	}

	multiple := 1.0
	switch trend {
	case "uptrend", "downtrend":
		multiple = 0.90
	}

	o := ohlc.New("test", now.Add(time.Duration(len(candles)-1)*candleDuration), candleDuration, false)
	o.Open = candles[len(candles)-2].Open.Mul(decimal.NewFromFloat(multiple))
	o.High = candles[len(candles)-2].High.Mul(decimal.NewFromFloat(multiple))
	o.Low = candles[len(candles)-2].Low.Mul(decimal.NewFromFloat(multiple))
	o.Close = candles[len(candles)-2].Close.Mul(decimal.NewFromFloat(multiple))
	o.ForceClose()
	candles[len(candles)-1] = o

	return candles
}
