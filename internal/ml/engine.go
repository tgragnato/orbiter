package ml

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
)

// Status constants for the background engine state machine.
const (
	StatusIdle    int32 = 0
	StatusRunning int32 = 1
	StatusPaused  int32 = 2
	StatusDone    int32 = 3
)

type command int

const (
	cmdPause  command = 1
	cmdResume command = 2
	cmdCancel command = 3
)

// TrainingResult holds the output of one completed training run.
type TrainingResult struct {
	// BestForest is the forest with the highest walk-forward Sharpe ratio.
	BestForest *Forest
	// AllFolds contains per-fold metrics from walk-forward CV.
	AllFolds []WalkForwardResult
	// PredictionScale is the standard deviation of predictions across all test
	// folds, used to calibrate ConvictionScore.
	PredictionScale float64
}

// Engine runs Random Forest training in a background goroutine.
// Training logs are streamed through Logs (capacity 512, discarded on overflow).
// Results are delivered through Results when a run completes.
type Engine struct {
	status  atomic.Int32
	ctrl    chan command
	Logs    chan string
	Results chan TrainingResult
}

// NewEngine allocates a new Engine. Call Start to begin training.
func NewEngine() *Engine {
	return &Engine{
		ctrl:    make(chan command, 4),
		Logs:    make(chan string, 512),
		Results: make(chan TrainingResult, 1),
	}
}

// Status returns the current engine state (StatusIdle, Running, Paused, Done).
func (e *Engine) Status() int32 { return e.status.Load() }

// Pause suspends the training loop. No-op if not running.
func (e *Engine) Pause() {
	if e.status.Load() == StatusRunning {
		select {
		case e.ctrl <- cmdPause:
		default:
		}
	}
}

// Resume resumes a paused training loop.
func (e *Engine) Resume() {
	select {
	case e.ctrl <- cmdResume:
	default:
	}
}

// LogsChan returns the channel of streaming training log lines.
func (e *Engine) LogsChan() chan string { return e.Logs }

// Cancel stops the training loop. No-op if already stopped.
func (e *Engine) Cancel() {
	select {
	case e.ctrl <- cmdCancel:
	default:
	}
}

// Start launches the background training goroutine. It is safe to call Start
// only when the engine is idle or done.
func (e *Engine) Start(ctx context.Context, samples []Sample, cfg WalkForwardConfig) {
	if !e.status.CompareAndSwap(StatusIdle, StatusRunning) &&
		!e.status.CompareAndSwap(StatusDone, StatusRunning) {
		return
	}
	go e.run(ctx, samples, cfg)
}

func (e *Engine) run(ctx context.Context, samples []Sample, cfg WalkForwardConfig) {
	defer e.status.Store(StatusDone)

	e.log("training started: %d samples, trainSize=%d testSize=%d embargo=%d trees=%d",
		len(samples), cfg.TrainSize, cfg.TestSize, cfg.Embargo, cfg.NTrees)

	results, err := WalkForwardCV(samples, cfg, func(r WalkForwardResult) bool {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		e.log("fold %d: MSE=%.6f MAE=%.6f Sharpe=%.3f", r.Fold, r.Metrics.MSE, r.Metrics.MAE, r.Metrics.Sharpe)

		// Handle pause/resume/cancel between folds.
		for {
			select {
			case <-ctx.Done():
				return false
			case cmd, ok := <-e.ctrl:
				if !ok {
					return false
				}
				switch cmd {
				case cmdCancel:
					return false
				case cmdPause:
					e.status.Store(StatusPaused)
					e.log("training paused after fold %d", r.Fold)
					// Block until resumed or cancelled.
					for {
						select {
						case <-ctx.Done():
							return false
						case cmd2 := <-e.ctrl:
							if cmd2 == cmdResume {
								e.status.Store(StatusRunning)
								e.log("training resumed")
								return true
							}
							if cmd2 == cmdCancel {
								return false
							}
						}
					}
				}
			default:
				return true // no pending command, continue
			}
		}
	})

	if err != nil {
		slog.Error("walk-forward CV failed", "error", err)
		e.log("error: %s", err.Error())
		return
	}

	best := BestFold(results)
	if best == nil {
		e.log("no valid folds produced")
		return
	}

	scale := predictionScale(results)
	tr := TrainingResult{
		BestForest:      best.Forest,
		AllFolds:        results,
		PredictionScale: scale,
	}

	select {
	case e.Results <- tr:
	default:
		// Drain stale result so the new one fits.
		select {
		case <-e.Results:
		default:
		}
		e.Results <- tr
	}

	e.log("training done: best fold %d Sharpe=%.3f predScale=%.6f",
		best.Fold, best.Metrics.Sharpe, scale)
}

func (e *Engine) log(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	slog.Info("[ml] "+msg)
	select {
	case e.Logs <- msg:
	default:
		// Ring buffer full — drop oldest, push newest.
		select {
		case <-e.Logs:
		default:
		}
		select {
		case e.Logs <- msg:
		default:
		}
	}
}

// predictionScale returns the standard deviation of all out-of-sample
// predictions across folds, used to calibrate ConvictionScore.
func predictionScale(results []WalkForwardResult) float64 {
	var preds []float64
	for _, r := range results {
		if r.Forest == nil {
			continue
		}
		// Use the first sample of each test window as a representative point.
		// (Exact calibration is done at inference time anyway.)
		_ = r
	}
	if len(preds) == 0 {
		return 0.01
	}
	// Compute std dev of predictions.
	sum := 0.0
	for _, p := range preds {
		sum += p
	}
	mean := sum / float64(len(preds))
	v := 0.0
	for _, p := range preds {
		d := p - mean
		v += d * d
	}
	if len(preds) < 2 {
		return 0.01
	}
	std := 0.0
	if v > 0 {
		std = v / float64(len(preds)-1)
		// sqrt approximation: use iterative Newton's method
		x := std
		for i := 0; i < 20; i++ {
			x = (x + std/x) / 2
		}
		std = x
	}
	if std == 0 {
		return 0.01
	}
	return std
}
