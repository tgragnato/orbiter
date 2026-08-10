package round

import (
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/ohlc"
)

func TestRoundnum_Value(t *testing.T) {
	t.Parallel()

	var now = time.Now()
	var rn = New()

	var testCases = []struct {
		price                  float64
		lowerRoundNumberWeak   float64
		lowerRoundNumberStrong float64
		upperRoundNumberWeak   float64
		upperRoundNumberStrong float64
	}{
		{
			price:                  0.23561,
			lowerRoundNumberWeak:   0.23,
			lowerRoundNumberStrong: 0.20,
			upperRoundNumberWeak:   0.24,
			upperRoundNumberStrong: 0.30,
		},
		{
			price:                  9.5,
			lowerRoundNumberWeak:   9.0,
			lowerRoundNumberStrong: 1.0,
			upperRoundNumberWeak:   10.0,
			upperRoundNumberStrong: 10.0,
		},
		{
			price:                  95,
			lowerRoundNumberWeak:   90,
			lowerRoundNumberStrong: 10,
			upperRoundNumberWeak:   100,
			upperRoundNumberStrong: 100,
		},
		{
			price:                  278,
			lowerRoundNumberWeak:   200,
			lowerRoundNumberStrong: 100,
			upperRoundNumberWeak:   300,
			upperRoundNumberStrong: 1000,
		},
		{
			price:                  1210,
			lowerRoundNumberWeak:   1200,
			lowerRoundNumberStrong: 1000,
			upperRoundNumberWeak:   1300,
			upperRoundNumberStrong: 10000,
		},
	}

	for _, tc := range testCases {
		o := ohlc.New("test", now, time.Minute, false)
		o.NewPrice(tc.price, o.Start)
		o.ForceClose()
		rn.Insert(o)

		rnValue, err := rn.Value()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rnValue) != 4 {
			t.Fatalf("expected %d, got %d", 4, len(rnValue))
		}
		if tc.lowerRoundNumberWeak != rnValue[LowerRoundNumberWeak] {
			t.Fatalf("expected %v, got %v", tc.lowerRoundNumberWeak, rnValue[LowerRoundNumberWeak])
		}
		if tc.lowerRoundNumberStrong != rnValue[LowerRoundNumberStrong] {
			t.Fatalf("expected %v, got %v", tc.lowerRoundNumberStrong, rnValue[LowerRoundNumberStrong])
		}
		if tc.upperRoundNumberWeak != rnValue[UpperRoundNumberWeak] {
			t.Fatalf("expected %v, got %v", tc.upperRoundNumberWeak, rnValue[UpperRoundNumberWeak])
		}
		if tc.upperRoundNumberStrong != rnValue[UpperRoundNumberStrong] {
			t.Fatalf("expected %v, got %v", tc.upperRoundNumberStrong, rnValue[UpperRoundNumberStrong])
		}
	}
}
