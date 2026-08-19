package stochrsi_test

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"

	stochrsi "github.com/tgragnato/orbiter/pkg/strategy/stochrsi"
)

const testInstrument = "CAD/USD"

func TestStochRSI_Name(t *testing.T) {
	t.Parallel()

	s := stochrsi.New("test")
	if strategy.NameStochRSI != s.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameStochRSI, s.Name())
	}
}

func TestStochRSI_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	s := stochrsi.New("test")
	currentTick := tick.New("test", time.Now(), 100, 100)

	toOpen, _, _ := s.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}

// signalLen is the number of trailing candles that move in the signal
// direction. It must be larger than stochK+stochD (5+2=7) so that the
// StochRSI K window sees RSI still rising/falling at the very end, which
// guarantees a non-degenerate (non-NaN) K/D value.
//
// The circular buffer holds bufferMultiplier*rsiPeriod = 3*14 = 42 prices.
// The remaining (42 - signalLen = 20) slots are filled with counter-trend
// candles that prime RSI in the opposite direction, ensuring a clear RSI
// transition and a non-zero K/D range every time GetAll is called.
const signalLen = 22

// generateSimpleCandles generates a series of 99 to 598 candles where the
// last signalLen bars always move in the signal direction (up for bullish,
// down for bearish) and all earlier bars move in the opposite direction.
//
// Because the circular buffer retains only the last 42 prices, the buffer
// always presents (42-signalLen) counter-trend prices followed by signalLen
// signal prices — regardless of total count. This keeps RSI in transition
// across the StochRSI K window, producing a reliable K/D in the overbought
// or oversold zone without degenerate 0/0 division.
func generateSimpleCandles(isBullish bool) []*ohlc.OHLC {
	count := 99 + rand.IntN(500)
	candles := make([]*ohlc.OHLC, count)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// sign is +1 for bullish, -1 for bearish.
	// Signal bars move with sign; counter-trend bars move against it.
	sign := 1.0
	if !isBullish {
		sign = -1.0
	}

	price := float64(count)

	for idx := 1; idx <= count; idx++ {
		if idx > count-signalLen {
			price += sign
		} else {
			price -= sign
		}

		o := ohlc.New("test", now.Add(time.Duration(idx)*time.Minute), time.Minute, false)
		o.NewPrice(price, o.Start)
		o.ForceClose()

		candles[idx-1] = o
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
			instrument:    testInstrument,
			closedCandles: generateSimpleCandles(true),
			want:          -1.0,
		},
		{
			name:          "Bearish Candle Series",
			instrument:    testInstrument,
			closedCandles: generateSimpleCandles(false),
			want:          1.0,
		},
		{
			name:       "Single Candle Series",
			instrument: testInstrument,
			closedCandles: []*ohlc.OHLC{
				{
					Open:  2.0,
					High:  3.0,
					Low:   1.0,
					Close: 1.5,
					Start: time.Now().Add(time.Hour),
					End:   time.Now().Add(2 * time.Hour),
				},
			},
			want: 0.0,
		},
		{
			name:          "Empty Candle Series",
			instrument:    testInstrument,
			closedCandles: []*ohlc.OHLC{},
			want:          0.0,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			d := stochrsi.New(testCase.instrument)
			for _, c := range testCase.closedCandles {
				d.OnWarmUpCandle(c)
			}

			got := d.Score(nil)
			if got != testCase.want {
				t.Errorf("Score() = %v, want %v", got, testCase.want)
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
					Open:  2.0),
					High:  3.0),
					Low:   1.0),
					Close: 1.5),
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
