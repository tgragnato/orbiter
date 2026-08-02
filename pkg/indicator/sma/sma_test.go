package sma

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

func TestSMA_Value(t *testing.T) {
	t.Parallel()

	var sma20 = New(21)

	total := 0
	prices := 0
	now := time.Now()
	for i := 1; i < 22; i++ {
		o := ohlc.New("test", now, time.Minute, false)
		total += i
		prices++
		o.NewPrice(decimal.NewFromFloat(float64(i)), o.Start)
		o.ForceClose()
		sma20.Insert(o)
		if i < 20 {
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
		sma20Value[Value] {
		t.Fatalf("expected %v, got %v", float64(total/prices), sma20Value[Value])
	}
}
