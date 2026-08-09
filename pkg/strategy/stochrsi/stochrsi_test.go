package stochrsi

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

func TestStochRSI_Name(t *testing.T) {
	t.Parallel()

	s := New("test")
	if strategy.NameStochRSI != s.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameStochRSI, s.Name())
	}
}

func TestStochRSI_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	s := New("test")
	currentTick := tick.New("test", time.Now(), decimal.NewFromFloat(100), decimal.NewFromFloat(100))

	toOpen, _, _ := s.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}

// generateSimpleCandles generates a linear series of 99 to 598 candles.
func generateSimpleCandles(isBullish bool) []*ohlc.OHLC {
	count := 99 + rand.IntN(500)
	candles := make([]*ohlc.OHLC, count)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 1; i <= count; i++ {
		var price float64
		if isBullish {
			price = float64(i)
		} else {
			price = float64(count - i + 1)
		}

		o := ohlc.New("test", now.Add(time.Duration(i)*time.Minute), time.Minute, false)
		o.NewPrice(decimal.NewFromFloat(price), o.Start)
		o.ForceClose()

		candles[i-1] = o
	}
	return candles
}

func TestRSI_Score(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		instrument    string
		closedCandles []*ohlc.OHLC
		want          float64
	}{
		{
			name:          "Bullish Candle Series",
			instrument:    "CAD/USD",
			closedCandles: generateSimpleCandles(true),
			want:          -1.0,
		},
		{
			name:          "Bearish Candle Series",
			instrument:    "CAD/USD",
			closedCandles: generateSimpleCandles(false),
			want:          1.0,
		},
		{
			name:       "Single Candle Series",
			instrument: "CAD/USD",
			closedCandles: []*ohlc.OHLC{
				{
					Open:  decimal.NewFromFloat(2.0),
					High:  decimal.NewFromFloat(3.0),
					Low:   decimal.NewFromFloat(1.0),
					Close: decimal.NewFromFloat(1.5),
					Start: time.Now().Add(time.Hour),
					End:   time.Now().Add(2 * time.Hour),
				},
			},
			want: 0.0,
		},
		{
			name:          "Empty Candle Series",
			instrument:    "CAD/USD",
			closedCandles: []*ohlc.OHLC{},
			want:          0.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := New(tt.instrument)
			for _, c := range tt.closedCandles {
				d.OnWarmUpCandle(c)
			}
			got := d.Score(nil)
			if got != tt.want {
				t.Errorf("Score() = %v, want %v", got, tt.want)
			}
		})
	}
}

/*func TestRSI_OnCandle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		instrument    string
		closedCandles []*ohlc.OHLC
		wantToClose   bool
	}{
		{
			name:          "Bullish Candle Series",
			instrument:    "CAD/USD",
			closedCandles: generateSimpleCandles(true),
			wantToClose:   true,
		},
		{
			name:          "Bearish Candle Series",
			instrument:    "CAD/USD",
			closedCandles: generateSimpleCandles(false),
			wantToClose:   false,
		},
		{
			name:       "Single Candle Series",
			instrument: "CAD/USD",
			closedCandles: []*ohlc.OHLC{
				{
					Open:  decimal.NewFromFloat(2.0),
					High:  decimal.NewFromFloat(3.0),
					Low:   decimal.NewFromFloat(1.0),
					Close: decimal.NewFromFloat(1.5),
					Start: time.Now().Add(time.Hour),
					End:   time.Now().Add(2 * time.Hour),
				},
			},
			wantToClose: false,
		},
		{
			name:          "Empty Candle Series",
			instrument:    "CAD/USD",
			closedCandles: []*ohlc.OHLC{},
			wantToClose:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := New(tt.instrument)
			for _, c := range tt.closedCandles {
				d.OnWarmUpCandle(c)
			}
			gotOpen, gotClose, gotToClose := d.OnCandle(tt.closedCandles)
			if (len(gotOpen)+len(gotClose)+len(gotToClose) <= 0) && tt.wantToClose {
				t.Errorf("OnCandle() gotOpen = %v, gotClose = %v, gotToClose = %v, want some != 0", gotOpen, gotClose, gotToClose)
			}
			if (len(gotOpen)+len(gotClose)+len(gotToClose) != 0) && !tt.wantToClose {
				t.Errorf("OnCandle() gotOpen = %v, gotClose = %v, gotToClose = %v, want all == 0", gotOpen, gotClose, gotToClose)
			}
		})
	}
}*/
