package sma_test

import (
	"strings"
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/indicator/sma"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

func TestSMA_Value(t *testing.T) {
	t.Parallel()

	var sma20 = sma.New(21)

	total := 0
	prices := 0

	now := time.Now()

	for idx := 1; idx < 22; idx++ {
		bar := ohlc.New("test", now, time.Minute, false)
		total += idx
		prices++

		bar.NewPrice(float64(idx), bar.Start)
		bar.ForceClose()
		sma20.Insert(bar)

		if idx < 20 {
			_, err := sma20.Value()

			if err == nil || !strings.Contains(err.Error(), "not enough") {
				t.Fatalf("expected error containing not enough, got %v", err)
			}
		}
	}

	sma20Value, err := sma20.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if float64(total/prices) !=
		sma20Value[sma.Value] {
		t.Fatalf("expected %v, got %v", float64(total/prices), sma20Value[sma.Value])
	}
}
