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

package adx_test

import (
	"errors"
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/indicator"
	"github.com/tgragnato/orbiter/pkg/indicator/adx"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

func TestADX_Bearish_Trend_Above_35(t *testing.T) {
	t.Parallel()

	var adx1 = adx.New(14)
	for i := 100; i > 0; i-- {
		adx1.Insert(generateCandle(float64(i)))
	}

	adxValue, err := adx1.Value()
	t.Logf("adx1 -> %f", adxValue[adx.Value])

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !(adxValue[adx.Value] > 35.0) {
		t.Fatalf("expected true")
	}
}

func TestADX_Bullish_Trend_Above_35(t *testing.T) {
	t.Parallel()

	var adx1 = adx.New(14)
	for i := range 100 {
		adx1.Insert(generateCandle(float64(i)))
	}

	adxValue, err := adx1.Value()
	t.Logf("adx1 -> %f", adxValue[adx.Value])

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if adxValue[adx.Value] <= 35.0 {
		t.Fatalf("expected true")
	}
}

func TestADX_NotEnoughCandles(t *testing.T) {
	t.Parallel()

	var adxIndicator = adx.New(14)
	adxIndicator.Insert(generateCandle(1))
	adxIndicator.Insert(generateCandle(2))

	_, err := adxIndicator.Value()
	if !errors.Is(err, indicator.ErrNotEnoughData) {
		t.Fatalf("expected error %v, got %v", indicator.ErrNotEnoughData, err)
	}
}

func generateCandle(price float64) *ohlc.OHLC {
	var o = ohlc.New("test", time.Now(), time.Minute, false)
	o.NewPrice(price, o.Start)
	o.ForceClose()

	return o
}
