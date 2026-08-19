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
