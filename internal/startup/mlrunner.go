package startup

import (
	"context"
	"errors"
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
//
// convictionReady is closed exactly once: either immediately when no active
// checkpoint exists, or after seedConvictionScores completes. Callers that must
// not read a cold conviction map (e.g. the first TAA evaluation) should block on
// it before proceeding.
type mlRunner struct {
	engine         *ml.Engine
	samples        func() []ml.Sample
	cfg            ml.WalkForwardConfig
	interval       time.Duration
	checkpoint     *ml.Checkpoint
	currentSamples func(context.Context) (map[string]ml.Sample, error)

	mu              sync.Mutex
	lastRun         time.Time
	trigger         chan struct{}
	conviction      sync.Map   // map[string]float64
	convictionReady chan struct{} // closed once conviction scores are seeded (or unavailable)
}

func newMLRunner(
	engine *ml.Engine,
	samples func() []ml.Sample,
	cfg ml.WalkForwardConfig,
	checkpoint *ml.Checkpoint,
	currentSamples func(context.Context) (map[string]ml.Sample, error),
) *mlRunner {
	return &mlRunner{
		engine:          engine,
		samples:         samples,
		cfg:             cfg,
		interval:        time.Hour,
		trigger:         make(chan struct{}, 1),
		checkpoint:      checkpoint,
		currentSamples:  currentSamples,
		convictionReady: make(chan struct{}),
	}
}

func (r *mlRunner) Status() int32         { return r.engine.Status() }
func (r *mlRunner) Pause()                { r.engine.Pause() }
func (r *mlRunner) Resume()               { r.engine.Resume() }
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

	// Initialise lastRun from the checkpoint timestamp before maybeStart checks
	// the 24-hour interval — otherwise lastRun is zero on every restart and a
	// full retrain fires even if one completed recently.
	// The conviction score population (network I/O) runs in a separate goroutine
	// so it never blocks the training scheduler. convictionReady is closed when
	// seeding finishes (or immediately when there is no active checkpoint) so that
	// the TAA engine's first Evaluate can safely block on it.
	if forest := r.initLastRunFromCheckpoint(ctx); forest != nil {
		go func() {
			r.seedConvictionScores(ctx, forest)
			close(r.convictionReady)
		}()
	} else {
		close(r.convictionReady)
	}
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
	if result.Forest == nil {
		return
	}

	if r.checkpoint != nil {
		var m ml.Metrics
		if best := ml.BestFold(result.AllFolds); best != nil {
			m = best.Metrics
		}
		if err := r.checkpoint.Save(ctx, "MAIN", result.Forest, m, true); err != nil {
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
	for sym := range samples {
		score := result.Forest.ConvictionScore(samples[sym].Features, result.PredictionScale)
		r.conviction.Store(sym, score)
	}
	slog.Info("ml: conviction scores updated", "symbols", len(samples))
}

// initLastRunFromCheckpoint queries the DB for the active checkpoint and
// initialises lastRun from its created_at timestamp. Returns the forest so the
// caller can seed conviction scores asynchronously; returns nil on any error.
func (r *mlRunner) initLastRunFromCheckpoint(ctx context.Context) *ml.Forest {
	if r.checkpoint == nil {
		return nil
	}
	forest, createdAt, err := r.checkpoint.LoadActive(ctx, "MAIN")
	if err != nil {
		if !errors.Is(err, ml.ErrNoActiveModel) {
			slog.Warn("ml: checkpoint load failed", "error", err)
		}
		return nil
	}
	r.mu.Lock()
	r.lastRun = createdAt
	r.mu.Unlock()
	return forest
}

// seedConvictionScores fetches current feature vectors and populates per-symbol
// conviction scores from the given forest. Runs asynchronously — involves
// network I/O and must not block the training scheduler.
func (r *mlRunner) seedConvictionScores(ctx context.Context, forest *ml.Forest) {
	if r.currentSamples == nil {
		return
	}
	samples, err := r.currentSamples(ctx)
	if err != nil {
		slog.Warn("ml: seed current samples failed", "error", err)
		return
	}
	for sym := range samples {
		score := forest.ConvictionScore(samples[sym].Features, 0.01)
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
