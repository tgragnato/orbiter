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

//nolint:testpackage // accesses unexported riskLevel method
package eo

import (
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/ohlc"
)

func Test_riskLevelHigh(t *testing.T) {
	t.Parallel()

	overlay := New()

	for i := range 100 {
		candle := generateCandle(float64(i))
		overlay.AddCandle(candle)
	}

	level := overlay.riskLevel()
	if int(RExtreme) != int(level) {
		t.Fatalf("expected %d, got %d", int(RExtreme), int(level))
	}
}

func Test_riskLevelLow(t *testing.T) {
	t.Parallel()

	overlay := New()

	for i := 100; i > 0; i-- {
		candle := generateCandle(float64(i))
		overlay.AddCandle(candle)
	}

	level := overlay.riskLevel()
	if int(RLow) != int(level) {
		t.Fatalf("expected %d, got %d", int(RLow), int(level))
	}
}

func generateCandle(diff float64) *ohlc.OHLC {
	now := time.Now()
	candle := ohlc.New("test", now, time.Minute, false)
	candle.NewPrice(10, now)
	candle.NewPrice(10+diff, now)
	candle.ForceClose()

	return candle
}
