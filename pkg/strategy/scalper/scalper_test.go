package scalper_test

import (
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/strategy/scalper"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// generateCandles creates a series of OHLC candles.
// direction: "long" or "short".
// count: number of candles.
func generateCandles(direction string, count int) []*ohlc.OHLC {
	candles := make([]*ohlc.OHLC, count)
	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	for candleIdx := range count {
		var openPrice, closePrice float64
		if direction == "long" {
			openPrice = float64(100 + candleIdx)
			closePrice = openPrice + 0.5
		} else { // short
			openPrice = float64(100 - candleIdx)
			closePrice = openPrice - 0.5
		}

		candles[candleIdx] = &ohlc.OHLC{
			Instrument: "",
			Open:       openPrice,
			High:       openPrice + 1.0,
			HighTime:   time.Time{},
			Low:        openPrice - 1.0,
			LowTime:    time.Time{},
			Close:      closePrice,
			Start:      start.Add(time.Duration(candleIdx) * time.Minute),
			End:        start.Add(time.Duration(candleIdx+1) * time.Minute),
			Duration:   0,
			Gaps:       false,
		}
	}

	return candles
}

func TestScalper_Name(t *testing.T) {
	t.Parallel()

	scalperInst := scalper.New("test")
	if strategy.NameScalper != scalperInst.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameScalper, scalperInst.Name())
	}
}

func TestScalper_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	scalperInst := scalper.New("test")
	currentTick := tick.New("test", time.Now(), 100, 100)

	toOpen, _, _ := scalperInst.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}

func TestScalper_Score(t *testing.T) {
	t.Parallel()

	scalperInst := scalper.New("test")

	tests := []struct {
		name          string
		candles       []*ohlc.OHLC
		expectedScore float64
	}{
		// Case 1: Last candle is Long (Buy direction). Opposite candles (Short) lead to positive score.
		// [Short x3, Long] -> Last is Long. 3 opposite (Short) / 3 total prior = 1.0
		{
			name:          "All Short followed by one Long",
			candles:       append(generateCandles("short", 3), generateCandles("long", 1)...),
			expectedScore: 1.0,
		},
		// [S, L, S, L] -> Last is Long. 2 opposite (S) / 3 total prior = 0.666...
		{
			name: "Alternating S-L-S-L",
			candles: []*ohlc.OHLC{
				generateCandles("short", 1)[0],
				generateCandles("long", 1)[0],
				generateCandles("short", 1)[0],
				generateCandles("long", 1)[0],
			},
			expectedScore: 2.0 / 3.0,
		},
		// Case 2: Last candle is Short (Sell direction). Opposite candles (Long) lead to negative score.
		// [Long x3, Short] -> Last is Short. 3 opposite (Long) / 3 total prior = 1.0 -> Score = -1.0
		{
			name:          "All Long followed by one Short",
			candles:       append(generateCandles("long", 3), generateCandles("short", 1)...),
			expectedScore: -1.0,
		},
		// Edge Cases
		{
			name:          "Single Candle (No history)",
			candles:       []*ohlc.OHLC{generateCandles("long", 1)[0]},
			expectedScore: 0.0,
		},
		// 0 opposite / 1 prior = 0
		{
			name:          "Two Candles Same Direction",
			candles:       generateCandles("long", 2),
			expectedScore: 0.0,
		},
		// Last is Long, 1 opposite (S) / 1 prior = 1.0
		{
			name:          "Two Candles Opposite Direction",
			candles:       []*ohlc.OHLC{generateCandles("short", 1)[0], generateCandles("long", 1)[0]},
			expectedScore: 1.0,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := scalperInst.Score(testCase.candles)
			if got != testCase.expectedScore {
				t.Errorf("Score() = %v, want %v", got, testCase.expectedScore)
			}
		})
	}
}
