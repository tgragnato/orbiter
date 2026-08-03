package startup

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/tgragnato/orbiter/internal/ml"
)

// mlRunner wraps ml.Engine with 24-hour auto-scheduling and implements both
// the signals.MLEngine and taa.ConvictionProvider interfaces.
//
// Training runs automatically when:
//   - The engine is idle/done AND the last run was more than interval ago.
//   - Trigger() is called (bypasses the interval check).
//
// After each training run the best forest is saved to PostgreSQL via checkpoint
// and per-symbol conviction scores are recomputed from current candle data.
type mlRunner struct {
	engine         *ml.Engine
	samples        func() []ml.Sample
	cfg            ml.WalkForwardConfig
	interval       time.Duration
	checkpoint     *ml.Checkpoint
	currentSamples func(context.Context) (map[string]ml.Sample, error)

	mu         sync.Mutex
	lastRun    time.Time
	trigger    chan struct{}
	conviction sync.Map // map[string]float64
}

func newMLRunner(
	engine *ml.Engine,
	samples func() []ml.Sample,
	cfg ml.WalkForwardConfig,
	checkpoint *ml.Checkpoint,
	currentSamples func(context.Context) (map[string]ml.Sample, error),
) *mlRunner {
	return &mlRunner{
		engine:         engine,
		samples:        samples,
		cfg:            cfg,
		interval:       24 * time.Hour,
		trigger:        make(chan struct{}, 1),
		checkpoint:     checkpoint,
		currentSamples: currentSamples,
	}
}

func (r *mlRunner) Status() int32        { return r.engine.Status() }
func (r *mlRunner) Pause()               { r.engine.Pause() }
func (r *mlRunner) Resume()              { r.engine.Resume() }
func (r *mlRunner) LogsChan() chan string { return r.engine.LogsChan() }

// Conviction implements taa.ConvictionProvider. Returns the most recently
// computed conviction score for symbol in [-1,+1], or 0 when no score exists.
func (r *mlRunner) Conviction(symbol string) float64 {
	if v, ok := r.conviction.Load(symbol); ok {
		return v.(float64)
	}
	return 0
}

// Symbols implements taa.SymbolProvider. Returns all symbols the ML engine has
// conviction scores for, in no particular order.
func (r *mlRunner) Symbols() []string {
	var syms []string
	r.conviction.Range(func(k, _ any) bool {
		syms = append(syms, k.(string))
		return true
	})
	return syms
}

// Trigger requests an immediate training run, ignoring the 24-hour interval.
// If training is already in progress the request is silently dropped.
func (r *mlRunner) Trigger() {
	select {
	case r.trigger <- struct{}{}:
	default:
	}
}

// run is the background scheduler goroutine. It also spawns a separate goroutine
// that drains Engine.Results so every completed training run is persisted and
// its conviction scores are applied to the TAA engine.
func (r *mlRunner) run(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case result := <-r.engine.Results:
				r.applyResult(ctx, result)
			}
		}
	}()

	r.maybeStart(ctx, true)

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.engine.Cancel()
			return
		case <-r.trigger:
			r.maybeStart(ctx, false)
		case <-ticker.C:
			r.maybeStart(ctx, true)
		}
	}
}

// applyResult persists the best forest to the checkpoint store and recomputes
// per-symbol conviction scores from the current feature vectors.
func (r *mlRunner) applyResult(ctx context.Context, result ml.TrainingResult) {
	if result.BestForest == nil {
		return
	}

	if r.checkpoint != nil {
		var m ml.Metrics
		if best := ml.BestFold(result.AllFolds); best != nil {
			m = best.Metrics
		}
		if err := r.checkpoint.Save(ctx, "MAIN", result.BestForest, m, true); err != nil {
			slog.Warn("ml: checkpoint save failed", "error", err)
		}
	}

	if r.currentSamples == nil {
		return
	}
	samples, err := r.currentSamples(ctx)
	if err != nil {
		slog.Warn("ml: current samples failed", "error", err)
		return
	}
	for sym, s := range samples {
		score := result.BestForest.ConvictionScore(s.Features, result.PredictionScale)
		r.conviction.Store(sym, score)
	}
	slog.Info("ml: conviction scores updated", "symbols", len(samples))
}

// seedFromCheckpoint loads the most recent active checkpoint and pre-populates
// per-symbol conviction scores. Must be called as a goroutine — fetching current
// candles involves network I/O and must not block startup.
func (r *mlRunner) seedFromCheckpoint(ctx context.Context) {
	if r.checkpoint == nil || r.currentSamples == nil {
		return
	}
	forest, err := r.checkpoint.LoadActive(ctx, "MAIN")
	if err != nil {
		if err != ml.ErrNoActiveModel {
			slog.Warn("ml: checkpoint load failed", "error", err)
		}
		return
	}
	samples, err := r.currentSamples(ctx)
	if err != nil {
		slog.Warn("ml: seed current samples failed", "error", err)
		return
	}
	for sym, s := range samples {
		score := forest.ConvictionScore(s.Features, 0.01)
		r.conviction.Store(sym, score)
	}
	slog.Info("ml: conviction seeded from checkpoint", "symbols", len(samples))
}

// maybeStart launches training when the engine is idle/done.
// When respectInterval is true it only starts if at least r.interval has
// elapsed since the last completed run; when false it starts immediately.
func (r *mlRunner) maybeStart(ctx context.Context, respectInterval bool) {
	status := r.engine.Status()
	if status != ml.StatusIdle && status != ml.StatusDone {
		return
	}

	if respectInterval {
		r.mu.Lock()
		elapsed := time.Since(r.lastRun)
		r.mu.Unlock()
		if elapsed < r.interval {
			return
		}
	}

	samples := r.samples()
	if len(samples) == 0 {
		return
	}

	r.mu.Lock()
	r.lastRun = time.Now()
	r.mu.Unlock()

	r.engine.Start(ctx, samples, r.cfg)
}
