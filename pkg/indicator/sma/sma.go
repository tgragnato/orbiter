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

package sma

import (
	"fmt"

	"github.com/tgragnato/orbiter/pkg/circularbuffer"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

// Value is the key used to store the SMA value in the result map.
const Value = "SMA_VALUE"

// SMA computes a simple moving average over a fixed window of OHLC close prices.
type SMA struct {
	cb *circularbuffer.CircularBuffer
}

// New creates a new SMA indicator with the given window size.
func New(size int) *SMA {
	return &SMA{
		cb: circularbuffer.New(size, size),
	}
}

// Insert adds the close price of a closed OHLC bar to the moving average window.
func (v *SMA) Insert(o *ohlc.OHLC) {
	if !o.Closed() {
		return
	}

	v.cb.Insert(o.Close)
}

// Value returns a map containing the current SMA value.
func (v *SMA) Value() (map[string]float64, error) {
	result := map[string]float64{}

	avg, avgErr := v.cb.Average()
	if avgErr != nil {
		return result, fmt.Errorf("sma average: %w", avgErr)
	}

	result[Value] = avg

	return result, nil
}

// ValueResultKeys returns the list of keys present in the Value result map.
func (v *SMA) ValueResultKeys() []string {
	return []string{Value}
}
