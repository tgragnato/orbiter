package trader

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"
	"github.com/sklinkert/at/internal/broker"
	"github.com/sklinkert/at/pkg/helper"
)

type PerformanceRecord struct {
	ID                         uint
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
	BacktestingID              string
	StrategyName               string
	Strategy                   string
	Instrument                 string
	CandleDuration             time.Duration
	TargetInPips               float64
	StopLossInPips             float64
	PerformanceTrigger         float64
	TotalPerformanceInPips     float64
	AVGPerformanceInPips       float64
	MaxAggregateDrawdownInPips float64
	MaxLossInPips              float64
	MaxLossInPercent           float64
	MaxWinInPercent            float64
	MaxWinInPips               float64
	TradesWinRationInPercent   float64
	Trades                     int
	TradesWin                  int
	TradesLoss                 int
	TradesLossLong             int
	TradesLossShort            int
	TradesLong                 int
	TradesShort                int
	MaxConsecutiveTradesLoss   uint
	MaxConcurrentPositions     int
	GitRev                     string
	Duration                   string
	FirstTrade                 time.Time
	LastTrade                  time.Time
	AVGTradeDurationInSeconds  float64
	TotalExposureInPercent     float64
	ChartHTML                  string
	BacktestingConfigJSON      string
	ClosedPositions            []broker.Position
	TotalTimeInMarket          time.Duration
	AVGTimeInMarket            time.Duration
}

const pipsFactor = 10000.0

func (tr *Trader) GetPerformanceRecords() ([]PerformanceRecord, error) {
	if tr.db == nil {
		return nil, errors.New("db is not configured")
	}

	var records []PerformanceRecord
	rows, err := tr.db.QueryContext(tr.ctx, `
		SELECT id, created_at, updated_at, backtesting_id, strategy_name, strategy,
		       instrument, candle_duration_ns, target_in_pips, stop_loss_in_pips,
		       performance_trigger, total_performance_in_pips, avg_performance_in_pips,
		       max_aggregate_drawdown_in_pips, max_loss_in_pips, max_loss_in_percent,
		       max_win_in_percent, max_win_in_pips, trades_win_ration_in_percent,
		       trades, trades_win, trades_loss, trades_loss_long, trades_loss_short,
		       trades_long, trades_short, max_consecutive_trades_loss,
		       max_concurrent_positions, git_rev, duration, first_trade, last_trade,
		       avg_trade_duration_in_seconds, total_exposure_in_percent, chart_html,
		       backtesting_config_json, total_time_in_market_ns, avg_time_in_market_ns
		FROM performance_records
		WHERE backtesting_id IS NOT NULL
		ORDER BY created_at DESC`)
	if err != nil {
		return records, err
	}
	defer rows.Close()

	for rows.Next() {
		rec, err := scanPerformanceRecord(rows)
		if err != nil {
			return records, err
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return records, err
	}

	return records, nil
}

func (tr *Trader) GetPerformanceRecordByID(backtestingID string) (PerformanceRecord, error) {
	if tr.db == nil {
		return PerformanceRecord{}, errors.New("db is not configured")
	}

	var record PerformanceRecord
	row := tr.db.QueryRowContext(tr.ctx, `
		SELECT id, created_at, updated_at, backtesting_id, strategy_name, strategy,
		       instrument, candle_duration_ns, target_in_pips, stop_loss_in_pips,
		       performance_trigger, total_performance_in_pips, avg_performance_in_pips,
		       max_aggregate_drawdown_in_pips, max_loss_in_pips, max_loss_in_percent,
		       max_win_in_percent, max_win_in_pips, trades_win_ration_in_percent,
		       trades, trades_win, trades_loss, trades_loss_long, trades_loss_short,
		       trades_long, trades_short, max_consecutive_trades_loss,
		       max_concurrent_positions, git_rev, duration, first_trade, last_trade,
		       avg_trade_duration_in_seconds, total_exposure_in_percent, chart_html,
		       backtesting_config_json, total_time_in_market_ns, avg_time_in_market_ns
		FROM performance_records
		WHERE backtesting_id = $1
		LIMIT 1`, backtestingID)

	rec, err := scanPerformanceRecord(row)
	if err != nil {
		return record, err
	}
	record = rec
	return record, nil
}

func (tr *Trader) totalTimeInMarket(closedPositions []broker.Position) (timeInMarket time.Duration) {
	for _, position := range closedPositions {
		timeInMarket += position.Duration()
	}
	return
}

func (tr *Trader) avgTimeInMarket(closedPositions []broker.Position) (avgTimeInMarket time.Duration) {
	totalTime := tr.totalTimeInMarket(closedPositions)
	positions := int64(len(closedPositions))
	if positions > 0 {
		avgTimeInMarket = totalTime / time.Duration(positions)
	}
	return
}

func (tr *Trader) candleDuration() (duration time.Duration) {
	if len(tr.closedCandles) > 0 {
		return tr.closedCandles[0].Duration
	}
	return
}

func (tr *Trader) tradesCounter(closedPositions []broker.Position, direction broker.BuyDirection) int {
	var trades int
	for _, position := range closedPositions {
		if position.BuyDirection == direction {
			trades++
		}
	}
	return trades
}

func (tr *Trader) maxWinInPips(closedPositions []broker.Position) float64 {
	var maxWin float64
	for _, position := range closedPositions {
		perf := position.PerformanceAbsolute(decimal.Decimal{}, decimal.Decimal{})
		if perf > maxWin {
			maxWin = perf
		}
	}
	return maxWin * pipsFactor
}

func (tr *Trader) maxWinInPercent(closedPositions []broker.Position) float64 {
	var maxWin float64
	for _, position := range closedPositions {
		perf := position.PerformanceInPercentage(decimal.Decimal{}, decimal.Decimal{})
		if perf > maxWin {
			maxWin = perf
		}
	}
	return maxWin
}

func (tr *Trader) maxLossInPips(closedPositions []broker.Position) float64 {
	var maxLoss float64
	for _, position := range closedPositions {
		perf := position.PerformanceAbsolute(decimal.Decimal{}, decimal.Decimal{})
		if perf < maxLoss {
			maxLoss = perf
		}
	}
	return maxLoss * pipsFactor
}

func (tr *Trader) maxLossInPercent(closedPositions []broker.Position) float64 {
	var maxLoss float64
	for _, position := range closedPositions {
		perf := position.PerformanceInPercentage(decimal.Decimal{}, decimal.Decimal{})
		if perf < maxLoss {
			maxLoss = perf
		}
	}
	return maxLoss
}

func (tr *Trader) tradesLossCounter(closedPositions []broker.Position, direction broker.BuyDirection) int {
	var trades int
	for _, position := range closedPositions {
		if position.BuyDirection == direction {
			perf := position.PerformanceInPercentage(decimal.Decimal{}, decimal.Decimal{})
			if perf < 0 {
				trades++
			}
		}
	}
	return trades
}

func (tr *Trader) tradesWinCounter(closedPositions []broker.Position, direction broker.BuyDirection) int {
	var trades int
	for _, position := range closedPositions {
		if position.BuyDirection == direction {
			perf := position.PerformanceInPercentage(decimal.Decimal{}, decimal.Decimal{})
			if perf >= 0 {
				trades++
			}
		}
	}
	return trades
}

func (tr *Trader) getMaxConsecutiveLossTrades(closedPositions []broker.Position) uint {
	var maxConsecutiveTradesLoss, currentTradesLoss uint
	for _, position := range closedPositions {
		perf := position.PerformanceInPercentage(decimal.Decimal{}, decimal.Decimal{})
		if perf < 0 {
			currentTradesLoss++
		} else {
			if currentTradesLoss > maxConsecutiveTradesLoss {
				maxConsecutiveTradesLoss = currentTradesLoss
				currentTradesLoss = 0
			}
		}
	}

	if currentTradesLoss > maxConsecutiveTradesLoss {
		maxConsecutiveTradesLoss = currentTradesLoss
	}
	return maxConsecutiveTradesLoss
}

func (tr *Trader) totalPerfInPips(closedPositions []broker.Position) decimal.Decimal {
	var totalPerfInPips decimal.Decimal
	for _, position := range closedPositions {
		perf := helper.Cent2Pips(decimal.NewFromFloat(position.PerformanceAbsolute(decimal.Decimal{}, decimal.Decimal{})))
		totalPerfInPips = totalPerfInPips.Add(perf)
	}
	return totalPerfInPips
}

func (tr *Trader) GetPerformanceRecord(chartHTML string) (*PerformanceRecord, error) {
	closedPositions, err := tr.GetClosedPositions()
	if err != nil {
		return nil, fmt.Errorf("unable to get closed positions: %w", err)
	}

	if len(closedPositions) == 0 {
		return nil, errors.New("no positions")
	}

	var totalPerfInPips = tr.totalPerfInPips(closedPositions)
	avgPerfInPips := totalPerfInPips.Div(decimal.NewFromFloat(float64(len(closedPositions))))
	avgPerfInPipsFloat, _ := avgPerfInPips.Float64()
	totalPerfInPipsFloat, _ := totalPerfInPips.Float64()
	maxAggregatedDrawdownFloat, _ := tr.MaxAggregatedDrawdownInPips.Float64()

	perf := &PerformanceRecord{
		BacktestingID:              tr.ID(),
		Instrument:                 tr.Instrument,
		StrategyName:               tr.strategy.Name(),
		Strategy:                   tr.strategy.String(),
		CandleDuration:             tr.candleDuration(),
		ChartHTML:                  chartHTML,
		TotalPerformanceInPips:     totalPerfInPipsFloat,
		AVGPerformanceInPips:       avgPerfInPipsFloat,
		Trades:                     len(closedPositions),
		TradesWin:                  tr.tradesWinCounter(closedPositions, broker.BuyDirectionLong) + tr.tradesWinCounter(closedPositions, broker.BuyDirectionShort),
		TradesLoss:                 tr.tradesLossCounter(closedPositions, broker.BuyDirectionLong) + tr.tradesLossCounter(closedPositions, broker.BuyDirectionShort),
		TradesLong:                 tr.tradesCounter(closedPositions, broker.BuyDirectionLong),
		TradesShort:                tr.tradesCounter(closedPositions, broker.BuyDirectionShort),
		TradesLossLong:             tr.tradesLossCounter(closedPositions, broker.BuyDirectionLong),
		TradesLossShort:            tr.tradesLossCounter(closedPositions, broker.BuyDirectionShort),
		MaxLossInPips:              tr.maxLossInPips(closedPositions),
		MaxLossInPercent:           tr.maxLossInPercent(closedPositions),
		MaxWinInPercent:            tr.maxWinInPercent(closedPositions),
		MaxWinInPips:               tr.maxWinInPips(closedPositions),
		MaxAggregateDrawdownInPips: maxAggregatedDrawdownFloat,
		MaxConcurrentPositions:     tr.maxConcurrentPositions,
		GitRev:                     tr.gitRev,
		FirstTrade:                 closedPositions[0].BuyTime,
		LastTrade:                  closedPositions[len(closedPositions)-1].BuyTime,
		Duration:                   time.Since(tr.StartTime).String(),
		MaxConsecutiveTradesLoss:   tr.getMaxConsecutiveLossTrades(closedPositions),
		ClosedPositions:            closedPositions,
		TotalTimeInMarket:          tr.totalTimeInMarket(closedPositions),
		AVGTimeInMarket:            tr.avgTimeInMarket(closedPositions),
		AVGTradeDurationInSeconds:  tr.totalTimeInMarket(closedPositions).Seconds() / float64(len(closedPositions)),
	}
	perf.TradesWinRationInPercent = float64(perf.TradesWin) * 100 / float64(perf.Trades)
	perf.TotalExposureInPercent = tr.totalExposureInPercent(perf.TotalTimeInMarket, perf.FirstTrade, perf.LastTrade)

	if (perf.TradesWin + perf.TradesLoss) != perf.Trades {
		return nil, fmt.Errorf("TradesWin(%d) + TradesLoss(%d) != Trades(%d)", perf.TradesWin, perf.TradesLoss, perf.Trades)
	}
	if (perf.TradesLong + perf.TradesShort) != perf.Trades {
		return nil, fmt.Errorf("TradesLong(%d) + TradesShort(%d) != Trades(%d)", perf.TradesLong, perf.TradesShort, perf.Trades)
	}

	return perf, nil
}

func (tr *Trader) totalExposureInPercent(totalTimeInMarket time.Duration, firstPrice, lastPrice time.Time) float64 {
	var totalTime = lastPrice.Sub(firstPrice)
	return float64(totalTimeInMarket) * 100 / float64(totalTime)
}

func (tr *Trader) Summary() {
	pr, err := tr.GetPerformanceRecord("")
	if err != nil {
		slog.Error("Cannot get performance record", "error", err)
		return
	}

	slog.Info(fmt.Sprintf("%25s: %s", "Instrument", pr.Instrument))
	slog.Info(fmt.Sprintf("%25s: %s", "Strategy", pr.Strategy))
	slog.Info(fmt.Sprintf("%25s: %s", "Candle duration", pr.CandleDuration))
	slog.Info(fmt.Sprintf("%25s: %s -> %s", "Period", pr.FirstTrade.Format("02.01.2006"), pr.LastTrade.Format("02.01.2006")))
	slog.Info(fmt.Sprintf("%25s: %d (%d long, %d short)", "Total positions", pr.Trades, pr.TradesLong, pr.TradesShort))
	slog.Info(fmt.Sprintf("%25s: %s (%.2f%%)", "Total time in market", pr.TotalTimeInMarket, pr.TotalExposureInPercent))
	slog.Info(fmt.Sprintf("%25s: %s", "AVG time in market", pr.AVGTimeInMarket))
	slog.Info(fmt.Sprintf("%25s: %d (%.2f%%)", "Profit positions", pr.TradesWin, pr.TradesWinRationInPercent))
	slog.Info(fmt.Sprintf("%25s: %d", "Loss positions", pr.TradesLoss))
	slog.Info(fmt.Sprintf("%25s: %d", "Loss positions long", pr.TradesLossLong))
	slog.Info(fmt.Sprintf("%25s: %d", "Loss positions short", pr.TradesLossShort))
	slog.Info(fmt.Sprintf("%25s: %.2f%% %.2f (%.2f pips)", "Max win", pr.MaxWinInPercent, pr.MaxWinInPips/pipsFactor, pr.MaxWinInPips))
	slog.Info(fmt.Sprintf("%25s: %.2f%% %.2f (%.2f pips)", "Max loss", pr.MaxLossInPercent, pr.MaxLossInPips/pipsFactor, pr.MaxLossInPips))
	slog.Info(fmt.Sprintf("%25s: %.2f (%.2f pips)", "Total performance", pr.TotalPerformanceInPips/pipsFactor, pr.TotalPerformanceInPips))
	slog.Info(fmt.Sprintf("%25s: %.2f (%.2f pips)", "AVG Performance", pr.AVGPerformanceInPips/pipsFactor, pr.AVGPerformanceInPips))
}

func (tr *Trader) SavePerformanceRecord(chartHTML string) error {
	if tr.db == nil {
		return errors.New("db is not configured")
	}

	performanceRecord, err := tr.GetPerformanceRecord(chartHTML)
	if err != nil {
		return err
	}

	id, err := tr.insertPerformanceRecord(tr.ctx, performanceRecord)
	if err != nil {
		return fmt.Errorf("cannot save PerformanceRecord to DB: %w", err)
	}
	performanceRecord.ID = uint(id)

	for _, pos := range performanceRecord.ClosedPositions {
		pos.PerformanceRecordID = performanceRecord.ID
		if err := tr.insertPosition(tr.ctx, pos); err != nil {
			slog.Error("Cannot save closed position to DB", "error", err)
		}
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPerformanceRecord(row rowScanner) (PerformanceRecord, error) {
	var rec PerformanceRecord
	var candleDurationNS int64
	var totalTimeInMarketNS int64
	var avgTimeInMarketNS int64

	err := row.Scan(
		&rec.ID,
		&rec.CreatedAt,
		&rec.UpdatedAt,
		&rec.BacktestingID,
		&rec.StrategyName,
		&rec.Strategy,
		&rec.Instrument,
		&candleDurationNS,
		&rec.TargetInPips,
		&rec.StopLossInPips,
		&rec.PerformanceTrigger,
		&rec.TotalPerformanceInPips,
		&rec.AVGPerformanceInPips,
		&rec.MaxAggregateDrawdownInPips,
		&rec.MaxLossInPips,
		&rec.MaxLossInPercent,
		&rec.MaxWinInPercent,
		&rec.MaxWinInPips,
		&rec.TradesWinRationInPercent,
		&rec.Trades,
		&rec.TradesWin,
		&rec.TradesLoss,
		&rec.TradesLossLong,
		&rec.TradesLossShort,
		&rec.TradesLong,
		&rec.TradesShort,
		&rec.MaxConsecutiveTradesLoss,
		&rec.MaxConcurrentPositions,
		&rec.GitRev,
		&rec.Duration,
		&rec.FirstTrade,
		&rec.LastTrade,
		&rec.AVGTradeDurationInSeconds,
		&rec.TotalExposureInPercent,
		&rec.ChartHTML,
		&rec.BacktestingConfigJSON,
		&totalTimeInMarketNS,
		&avgTimeInMarketNS,
	)
	if err != nil {
		return PerformanceRecord{}, err
	}

	rec.CandleDuration = time.Duration(candleDurationNS)
	rec.TotalTimeInMarket = time.Duration(totalTimeInMarketNS)
	rec.AVGTimeInMarket = time.Duration(avgTimeInMarketNS)

	return rec, nil
}

func (tr *Trader) insertPerformanceRecord(ctx context.Context, rec *PerformanceRecord) (uint64, error) {
	var id int64
	err := tr.db.QueryRowContext(ctx, `
		INSERT INTO performance_records (
			backtesting_id, strategy_name, strategy, instrument, candle_duration_ns,
			target_in_pips, stop_loss_in_pips, performance_trigger, total_performance_in_pips,
			avg_performance_in_pips, max_aggregate_drawdown_in_pips, max_loss_in_pips,
			max_loss_in_percent, max_win_in_percent, max_win_in_pips,
			trades_win_ration_in_percent, trades, trades_win, trades_loss, trades_loss_long,
			trades_loss_short, trades_long, trades_short, max_consecutive_trades_loss,
			max_concurrent_positions, git_rev, duration, first_trade, last_trade,
			avg_trade_duration_in_seconds, total_exposure_in_percent, chart_html,
			backtesting_config_json, total_time_in_market_ns, avg_time_in_market_ns
		) VALUES (
			$1,$2,$3,$4,$5,
			$6,$7,$8,$9,
			$10,$11,$12,
			$13,$14,$15,
			$16,$17,$18,$19,$20,
			$21,$22,$23,$24,
			$25,$26,$27,$28,$29,
			$30,$31,$32,
			$33,$34,$35
		)
		RETURNING id`,
		rec.BacktestingID,
		rec.StrategyName,
		rec.Strategy,
		rec.Instrument,
		int64(rec.CandleDuration),
		rec.TargetInPips,
		rec.StopLossInPips,
		rec.PerformanceTrigger,
		rec.TotalPerformanceInPips,
		rec.AVGPerformanceInPips,
		rec.MaxAggregateDrawdownInPips,
		rec.MaxLossInPips,
		rec.MaxLossInPercent,
		rec.MaxWinInPercent,
		rec.MaxWinInPips,
		rec.TradesWinRationInPercent,
		rec.Trades,
		rec.TradesWin,
		rec.TradesLoss,
		rec.TradesLossLong,
		rec.TradesLossShort,
		rec.TradesLong,
		rec.TradesShort,
		rec.MaxConsecutiveTradesLoss,
		rec.MaxConcurrentPositions,
		rec.GitRev,
		rec.Duration,
		rec.FirstTrade,
		rec.LastTrade,
		rec.AVGTradeDurationInSeconds,
		rec.TotalExposureInPercent,
		rec.ChartHTML,
		rec.BacktestingConfigJSON,
		int64(rec.TotalTimeInMarket),
		int64(rec.AVGTimeInMarket),
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return uint64(id), nil
}

func (tr *Trader) insertPosition(ctx context.Context, pos broker.Position) error {
	_, err := tr.db.ExecContext(ctx, `
		INSERT INTO positions (
			performance_record_id, reference, instrument, buy_price, buy_time, buy_direction,
			sell_price, sell_time, target_price, stop_loss_price, size, ohlc_age_on_buy_ns,
			candle_buy_time, candle_sell_time, max_surge, max_drawdown,
			today_performance_in_percent, gap_to_sma
		) VALUES (
			$1,$2,$3,$4,$5,$6,
			$7,$8,$9,$10,$11,$12,
			$13,$14,$15,$16,
			$17,$18
		)`,
		pos.PerformanceRecordID,
		pos.Reference,
		pos.Instrument,
		pos.BuyPrice.String(),
		pos.BuyTime,
		int(pos.BuyDirection),
		pos.SellPrice.String(),
		pos.SellTime,
		pos.TargetPrice.String(),
		pos.StopLossPrice.String(),
		pos.Size,
		int64(pos.OHLCAgeOnBuy),
		pos.CandleBuyTime,
		pos.CandleSellTime,
		pos.MaxSurge,
		pos.MaxDrawdown,
		pos.TodayPerformanceInPercent.String(),
		pos.GapToSMA.String(),
	)
	if err != nil {
		return err
	}
	return nil
}

var _ rowScanner = (*sql.Row)(nil)
