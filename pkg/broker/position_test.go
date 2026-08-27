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

package broker_test

import (
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/broker"
)

func TestPerformanceInPercentage(t *testing.T) {
	t.Parallel()

	currentPrice := 2.0

	// Long
	position := broker.Position{
		BuyPrice:                  1.0,
		BuyDirection:              broker.BuyDirectionLong,
		PerformanceRecordID:       0,
		Reference:                 "",
		Instrument:                "",
		BuyTime:                   time.Time{},
		SellPrice:                 0,
		SellTime:                  time.Time{},
		TargetPrice:               0,
		StopLossPrice:             0,
		Size:                      0,
		OHLCAgeOnBuy:              0,
		CandleBuyTime:             time.Time{},
		CandleSellTime:            time.Time{},
		MaxSurge:                  0,
		MaxDrawdown:               0,
		TodayPerformanceInPercent: 0,
		GapToSMA:                  0,
	}

	perf := position.PerformanceInPercentage(currentPrice, currentPrice)

	if perf != 100 {
		t.Fatalf("expected %v, got %v", 100, perf)
	}

	// Short
	currentPrice = 1.0
	position = broker.Position{
		BuyPrice:                  0.0,
		BuyDirection:              broker.BuyDirectionShort,
		PerformanceRecordID:       0,
		Reference:                 "",
		Instrument:                "",
		BuyTime:                   time.Time{},
		SellPrice:                 0,
		SellTime:                  time.Time{},
		TargetPrice:               0,
		StopLossPrice:             0,
		Size:                      0,
		OHLCAgeOnBuy:              0,
		CandleBuyTime:             time.Time{},
		CandleSellTime:            time.Time{},
		MaxSurge:                  0,
		MaxDrawdown:               0,
		TodayPerformanceInPercent: 0,
		GapToSMA:                  0,
	}

	perf = position.PerformanceInPercentage(currentPrice, currentPrice)

	if perf != -100 {
		t.Fatalf("expected %v, got %v", -100, perf)
	}
}

func TestPerformanceAbsolute(t *testing.T) {
	t.Parallel()

	currentPrice := 2.0

	// Long
	position := broker.Position{
		BuyPrice:                  1.0,
		BuyDirection:              broker.BuyDirectionLong,
		Size:                      1.00,
		PerformanceRecordID:       0,
		Reference:                 "",
		Instrument:                "",
		BuyTime:                   time.Time{},
		SellPrice:                 0,
		SellTime:                  time.Time{},
		TargetPrice:               0,
		StopLossPrice:             0,
		OHLCAgeOnBuy:              0,
		CandleBuyTime:             time.Time{},
		CandleSellTime:            time.Time{},
		MaxSurge:                  0,
		MaxDrawdown:               0,
		TodayPerformanceInPercent: 0,
		GapToSMA:                  0,
	}

	perf := position.PerformanceAbsolute(currentPrice, currentPrice)

	if perf != 1.0 {
		t.Fatalf("expected %v, got %v", 1.0, perf)
	}

	// Short
	position = broker.Position{
		BuyPrice:                  1.0,
		BuyDirection:              broker.BuyDirectionShort,
		Size:                      1.00,
		PerformanceRecordID:       0,
		Reference:                 "",
		Instrument:                "",
		BuyTime:                   time.Time{},
		SellPrice:                 0,
		SellTime:                  time.Time{},
		TargetPrice:               0,
		StopLossPrice:             0,
		OHLCAgeOnBuy:              0,
		CandleBuyTime:             time.Time{},
		CandleSellTime:            time.Time{},
		MaxSurge:                  0,
		MaxDrawdown:               0,
		TodayPerformanceInPercent: 0,
		GapToSMA:                  0,
	}

	perf = position.PerformanceAbsolute(currentPrice, currentPrice)

	if perf != -1.0 {
		t.Fatalf("expected %v, got %v", -1.0, perf)
	}
}
