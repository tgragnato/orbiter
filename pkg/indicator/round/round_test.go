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

package round_test

import (
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/indicator/round"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

//nolint:funlen // test table is intentionally long
func TestRoundnum_Value(t *testing.T) {
	t.Parallel()

	now := time.Now()
	roundNum := round.New()

	testCases := []struct {
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

	for _, testCase := range testCases {
		candle := ohlc.New("test", now, time.Minute, false)
		candle.NewPrice(testCase.price, candle.Start)
		candle.ForceClose()
		roundNum.Insert(candle)

		rnValue, err := roundNum.Value()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(rnValue) != 4 {
			t.Fatalf("expected %d, got %d", 4, len(rnValue))
		}

		if testCase.lowerRoundNumberWeak != rnValue[round.LowerRoundNumberWeak] {
			t.Fatalf("expected %v, got %v", testCase.lowerRoundNumberWeak, rnValue[round.LowerRoundNumberWeak])
		}

		if testCase.lowerRoundNumberStrong != rnValue[round.LowerRoundNumberStrong] {
			t.Fatalf("expected %v, got %v", testCase.lowerRoundNumberStrong, rnValue[round.LowerRoundNumberStrong])
		}

		if testCase.upperRoundNumberWeak != rnValue[round.UpperRoundNumberWeak] {
			t.Fatalf("expected %v, got %v", testCase.upperRoundNumberWeak, rnValue[round.UpperRoundNumberWeak])
		}

		if testCase.upperRoundNumberStrong != rnValue[round.UpperRoundNumberStrong] {
			t.Fatalf("expected %v, got %v", testCase.upperRoundNumberStrong, rnValue[round.UpperRoundNumberStrong])
		}
	}
}
