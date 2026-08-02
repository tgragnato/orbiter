package trader

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/internal/broker"
	"github.com/tgragnato/orbiter/internal/signal"
	"github.com/tgragnato/orbiter/internal/strategy"
	"github.com/tgragnato/orbiter/internal/trader/signalexec"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/tick"
)

type Trader struct {
	ctx                         context.Context
	StartTime                   time.Time
	Instrument                  string
	TickChan                    chan tick.Tick
	running                     bool
	clog                        *slog.Logger
	broker                      broker.Broker
	strategy                    strategy.Strategy
	persistTickData             bool
	persistCandleData           bool
	today                       *ohlc.OHLC
	maxConcurrentPositions      int
	MaxAggregatedDrawdownInPips decimal.Decimal
	reversedPerformanceInPips   map[ohlc.OHLC]float64
	db                          *sql.DB
	positionBuyTime             map[string]time.Time
	openCandles                 []*ohlc.OHLC // strategy's candle + today's candle
	closedCandles               []*ohlc.OHLC
	lastReceivedTick            *tick.Tick
	gitRev                      string
	candleSubscribers           []CandleSubscriber
	positionSubscribers         []PositionSubscriber
	orderSubscribers            []OrderSubscriber
	closedPositionReferences    map[string]bool
	currencyCode                string
	signalDispatcher            signal.Dispatcher
	signalOrchestrator          *signalexec.Orchestrator
	sync.Mutex
}

type PositionSubscriber interface {
	OnPosition(position broker.Position)
}

type CandleSubscriber interface {
	OnCandle(candle ohlc.OHLC)
}

type OrderSubscriber interface {
	OnOrder(order broker.Order)
}

type Option func(*Trader)

func WithBroker(broker broker.Broker) Option {
	return func(trader *Trader) {
		trader.broker = broker
	}
}

func WithCandleSubscription(subscriber CandleSubscriber) Option {
	return func(trader *Trader) {
		trader.candleSubscribers = append(trader.candleSubscribers, subscriber)
	}
}

func WithPositionSubscription(subscriber PositionSubscriber) Option {
	return func(trader *Trader) {
		trader.positionSubscribers = append(trader.positionSubscribers, subscriber)
	}
}

func WithPersistTickData(persist bool) Option {
	return func(trader *Trader) {
		trader.persistTickData = persist
	}
}

func WithPersistCandleData(persist bool) Option {
	return func(trader *Trader) {
		trader.persistCandleData = persist
	}
}

func WithStrategy(strategy strategy.Strategy) Option {
	return func(trader *Trader) {
		trader.strategy = strategy
	}
}

func WithCurrencyCode(currencyCode string) Option {
	return func(trader *Trader) {
		trader.currencyCode = currencyCode
	}
}

func WithFeedStoredCandles(strategy strategy.Strategy) Option {
	return func(trader *Trader) {
		var limit = int(strategy.GetWarmUpCandleAmount())
		var candlePeriod = strategy.GetCandleDuration()

		slog.Info("searching for warmup candles", "period", candlePeriod)

		candles, err := trader.loadWarmUpCandles(trader.ctx, trader.Instrument, candlePeriod, limit)
		if err != nil {
			slog.Error("fetching stored candles failed, skipping warm-up", "error", err)
			return
		}
		sort.Sort(ohlc.OHLCList(candles))

		slog.Info("sending candles to strategy for warm-up", "count", len(candles))
		for _, candle := range candles {
			candle.ForceClose()
			strategy.OnWarmUpCandle(&candle)
		}
	}
}

func WithSignalDispatcher(dispatcher signal.Dispatcher) Option {
	return func(trader *Trader) {
		trader.signalDispatcher = dispatcher
	}
}

func New(ctx context.Context, instrument, gitRev string, db *sql.DB, options ...Option) *Trader {
	var clog = slog.With(
		"INSTRUMENT", instrument,
		"GIT_REV", gitRev,
	)
	tr := &Trader{
		ctx:                       ctx,
		Instrument:                instrument,
		StartTime:                 time.Now(),
		clog:                      clog,
		TickChan:                  make(chan tick.Tick),
		reversedPerformanceInPips: make(map[ohlc.OHLC]float64),
		positionBuyTime:           make(map[string]time.Time),
		closedPositionReferences:  make(map[string]bool),
		gitRev:                    gitRev,
		db:                        db,
		currencyCode:              "USD", // default
	}

	for _, option := range options {
		option(tr)
	}

	if tr.signalDispatcher == nil {
		tr.signalDispatcher = signal.NewMemoryDispatcher()
	}
	tr.signalOrchestrator = signalexec.New(tr.signalDispatcher, tr.clog)

	if tr.db == nil {
		if tr.persistTickData || tr.persistCandleData {
			slog.Error("persistence requested but no DB given, disabling persistence")
			tr.persistTickData = false
			tr.persistCandleData = false
		}
	} else {
		if err := tr.ensureSchema(ctx); err != nil {
			slog.Error("db schema initialization failed, disabling persistence", "error", err)
			tr.persistTickData = false
			tr.persistCandleData = false
			tr.db = nil
		}
	}

	return tr
}

func (tr *Trader) ID() string {
	return fmt.Sprintf("rev_%s_strategy_%s", tr.gitRev, tr.strategy.Name())
}

func (tr *Trader) Start() error {
	if tr.running {
		return errors.New("already running")
	}
	tr.running = true
	tr.clog.Info("Starting trader")

	go tr.receiveTicks()
	tr.broker.ListenToPriceFeed(tr.TickChan)

	return nil
}

func (tr *Trader) Stop() error {
	if !tr.running {
		return errors.New("already stopped")
	}
	tr.clog.Info("Stopping trader")

	tr.Lock()
	defer tr.Unlock()

	close(tr.TickChan)
	tr.running = false
	tr.printPositionPerformanceByNotes()

	return nil
}

func (tr *Trader) GetClosedPositions() ([]broker.Position, error) {
	positions, err := tr.broker.GetClosedPositions()
	if err != nil {
		return []broker.Position{}, err
	}
	for i := range positions {
		positions[i].CandleBuyTime = tr.positionBuyTime[positions[i].Reference]
	}
	return positions, nil
}

func (tr *Trader) processTodayCandle(currentTick tick.Tick) {
	const eodPeriod = time.Hour * 24 * 1 // 1d

	if tr.today == nil || tr.today.Start.Day() != currentTick.Datetime.Day() {
		if tr.today != nil {
			tr.today.ForceClose()
		}
		tr.today = ohlc.New(tr.Instrument, currentTick.Datetime, eodPeriod, false)
	}
	tr.today.NewPrice(currentTick.Bid, currentTick.Datetime)
}

func (tr *Trader) getOpenPositions() ([]broker.Position, error) {
	positions, err := tr.broker.GetOpenPositions()
	if err != nil {
		return []broker.Position{}, err
	}
	for i := range positions {
		positions[i].CandleBuyTime = tr.positionBuyTime[positions[i].Reference]
	}
	return positions, nil
}

func (tr *Trader) persistTick(t tick.Tick) {
	if err := tr.insertTick(tr.ctx, t); err != nil {
		slog.Error("cannot persist tick", "tick", t, "error", err)
	}
}

func (tr *Trader) receiveTicks() {
	for currentTick := range tr.TickChan {
		if tr.persistTickData {
			go tr.persistTick(currentTick)
		}

		if err := currentTick.Validate(); err != nil {
			tr.clog.Debug("invalid tick data received", "tick", currentTick, "error", err)
			continue
		}

		tr.Lock()
		tr.processTodayCandle(currentTick)
		tr.processTick(currentTick)
		tr.Unlock()
	}
}

func (tr *Trader) processTick(currentTick tick.Tick) {
	var closedCandles = tr.processTickByOpenCandles(currentTick)

	tr.strategy.OnTick(currentTick)

	for _, closedCandle := range closedCandles {
		tr.processClosedCandle(closedCandle, currentTick)
	}
}

func (tr *Trader) processClosedCandle(closedCandle *ohlc.OHLC, currentTick tick.Tick) {
	tr.clog.Debug("processing closed candle", "candle", closedCandle)

	if !closedCandle.HasPriceData() {
		tr.clog.Debug("candle has missing price data, skipping", "candle", closedCandle)
		return
	}

	if tr.strategy.GetCandleDuration() != closedCandle.Duration {
		return
	}

	// Orders
	openOrders, err := tr.broker.GetOpenOrders()
	if err != nil {
		tr.clog.Error("Cannot get open orders", "error", err)
		return
	}
	tr.strategy.OnOrder(openOrders)

	// Positions
	openPositions, err := tr.getOpenPositions()
	if err != nil {
		tr.clog.Error("Cannot get open positions", "error", err)
		return
	}
	closedPositions, err := tr.GetClosedPositions()
	if err != nil {
		tr.clog.Error("Cannot get closed positions", "error", err)
		return
	}
	tr.detectClosedPositions(closedPositions)
	tr.processOpenPositions(closedCandle, openPositions)
	tr.strategy.OnPosition(openPositions, closedPositions)

	// Candle
	toOpen, toClose, toClosePositions := tr.strategy.OnCandle(tr.closedCandles)
	tr.processClosableOrders(toClose)
	tr.processClosablePositions(toClosePositions)
	tr.processOrders(toOpen)

	for _, subscriber := range tr.candleSubscribers {
		subscriber.OnCandle(*closedCandle)
	}
}

func (tr *Trader) processClosableOrders(orders []broker.Order) {
	tr.signalExec().DispatchClosableOrders(orders)
}

func (tr *Trader) processOpenPositions(candle *ohlc.OHLC, openPositions []broker.Position) {
	for _, openPosition := range openPositions {
		_, exists := tr.positionBuyTime[openPosition.Reference]
		if !exists {
			tr.positionBuyTime[openPosition.Reference] = candle.Start
		}
	}
}

func (tr *Trader) processClosablePositions(toClose []broker.Position) {
	tr.signalExec().DispatchClosablePositions(toClose)
}

// processOrders dispatches buy signals for new orders.
func (tr *Trader) processOrders(toOpen []broker.Order) {
	tr.signalExec().DispatchOpenOrders(toOpen, tr.currencyCode, func(order broker.Order) {
		for _, subscriber := range tr.orderSubscribers {
			subscriber.OnOrder(order)
		}
	})
}

func (tr *Trader) signalExec() *signalexec.Orchestrator {
	if tr.signalOrchestrator == nil {
		dispatcher := tr.signalDispatcher
		if dispatcher == nil {
			dispatcher = signal.NewMemoryDispatcher()
			tr.signalDispatcher = dispatcher
		}
		tr.signalOrchestrator = signalexec.New(dispatcher, tr.clog)
	}
	return tr.signalOrchestrator
}

func (tr *Trader) processTickByOpenCandles(currentTick tick.Tick) (closedCandles []*ohlc.OHLC) {
	var stillOpenCandles []*ohlc.OHLC

	defer func() {
		lastReceivedTick := currentTick
		tr.lastReceivedTick = &lastReceivedTick
		tr.openCandles = stillOpenCandles
	}()

	if len(tr.openCandles) == 0 {
		candle := ohlc.New(tr.Instrument, currentTick.Datetime, tr.strategy.GetCandleDuration(), true)
		tr.openCandles = append(tr.openCandles, candle)

		if tr.persistCandleData && tr.strategy.GetCandleDuration() != time.Hour*24 {
			candle := ohlc.New(tr.Instrument, currentTick.Datetime, time.Hour*24, true)
			tr.openCandles = append(tr.openCandles, candle)
		}
	}

	for _, candle := range tr.openCandles {
		switch candle.Duration {
		case time.Hour * 24:
			if tr.lastReceivedTick != nil && tr.lastReceivedTick.Datetime.Day() != currentTick.Datetime.Day() {
				candle.ForceClose()
			}
		case time.Hour:
			if tr.lastReceivedTick != nil && tr.lastReceivedTick.Datetime.Hour() != currentTick.Datetime.Hour() {
				candle.ForceClose()
			}
		}

		isOpen := candle.NewPrice(currentTick.Price(), currentTick.Datetime)
		if isOpen {
			stillOpenCandles = append(stillOpenCandles, candle)
			continue
		}

		newCandle := tr.closeCandle(currentTick, candle)
		stillOpenCandles = append(stillOpenCandles, newCandle)
		closedCandles = append(closedCandles, candle)
	}
	return
}

func (tr *Trader) closeCandle(tick tick.Tick, candle *ohlc.OHLC) (newCandle *ohlc.OHLC) {
	tr.closedCandles = append(tr.closedCandles, candle)

	var candlesToKeep = 100
	if len(tr.closedCandles) > candlesToKeep {
		tr.closedCandles = tr.closedCandles[len(tr.closedCandles)-candlesToKeep:]
	}

	if tr.db != nil && tr.persistCandleData {
		go func() {
			if err := candle.Store(tr.ctx, tr.db); err != nil {
				tr.clog.Error("failed to store OHLC", "candle", candle, "error", err)
			}
		}()
	}

	// Replace closed OHLC from openOHLCs list
	openCandle := ohlc.New(candle.Instrument, tick.Datetime, candle.Duration, true)
	openCandle.NewPrice(tick.Price(), tick.Datetime)
	return openCandle
}

func (tr *Trader) detectClosedPositions(brokerClosedPositions []broker.Position) {
	for _, closedByBroker := range brokerClosedPositions {
		_, exists := tr.closedPositionReferences[closedByBroker.Reference]
		if !exists {
			tr.closePosition(closedByBroker)
			tr.closedPositionReferences[closedByBroker.Reference] = true
		}
	}
}

func (tr *Trader) closePosition(position broker.Position) {
	for _, subscriber := range tr.positionSubscribers {
		subscriber.OnPosition(position)
	}
}
