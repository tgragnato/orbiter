package signalexec

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/tgragnato/orbiter/internal/broker"
	"github.com/tgragnato/orbiter/internal/signal"
)

// Orchestrator converts trading intents into UI-consumable signal messages.
type Orchestrator struct {
	dispatcher signal.Dispatcher
	logger     *slog.Logger
	now        func() time.Time
}

// New creates a signal execution orchestrator.
func New(dispatcher signal.Dispatcher, logger *slog.Logger) *Orchestrator {
	if logger == nil {
		logger = slog.Default()
	}
	return &Orchestrator{
		dispatcher: dispatcher,
		logger:     logger,
		now:        time.Now,
	}
}

// DispatchClosableOrders publishes cancellation messages for open orders.
func (o *Orchestrator) DispatchClosableOrders(orders []broker.Order) {
	for _, order := range orders {
		msg := signal.NewCancelOrderMessage(o.now(), order)
		if err := o.dispatcher.Dispatch(msg); err != nil {
			o.logger.Error("Unable to dispatch order cancellation signal", "error", err, "OrderID", order.ID)
		}
	}
}

// DispatchClosablePositions publishes sell messages for open positions.
func (o *Orchestrator) DispatchClosablePositions(positions []broker.Position) {
	for _, position := range positions {
		msg := signal.NewSellMessage(o.now(), position)
		if err := o.dispatcher.Dispatch(msg); err != nil {
			o.logger.Error("Unable to dispatch sell signal", "error", err, "Reference", position.Reference)
		}
	}
}

// DispatchOpenOrders publishes buy messages for newly opened orders.
func (o *Orchestrator) DispatchOpenOrders(orders []broker.Order, currencyCode string, onDispatched func(order broker.Order)) {
	for _, order := range orders {
		order.CurrencyCode = currencyCode

		msg := signal.NewBuyMessage(o.now(), order)
		if err := o.dispatcher.Dispatch(msg); err != nil {
			o.logger.Error(fmt.Sprintf("Unable to dispatch buy signal: %+v", order), "error", err)
			continue
		}

		o.logger.Info(fmt.Sprintf("Signal queued: %s", msg.Summary))
		if onDispatched != nil {
			onDispatched(order)
		}
	}
}
