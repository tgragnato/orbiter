package startup

import (
	"context"
	"sync"
	"time"

	"github.com/tgragnato/orbiter/internal/ml"
)

// mlRunner wraps ml.Engine with 24-hour auto-scheduling and implements the
// signals.MLEngine interface so it can be handed directly to the TUI.
//
// Training runs automatically when:
//   - The engine is idle/done AND the last run was more than interval ago.
//   - Trigger() is called (bypasses the interval check).
//
// If no samples are available the scheduler stays idle without error.
type mlRunner struct {
	engine   *ml.Engine
	samples  func() []ml.Sample
	cfg      ml.WalkForwardConfig
	interval time.Duration

	mu      sync.Mutex
	lastRun time.Time
	trigger chan struct{}
}

func newMLRunner(engine *ml.Engine, samples func() []ml.Sample, cfg ml.WalkForwardConfig) *mlRunner {
	return &mlRunner{
		engine:   engine,
		samples:  samples,
		cfg:      cfg,
		interval: 24 * time.Hour,
		trigger:  make(chan struct{}, 1),
	}
}

func (r *mlRunner) Status() int32        { return r.engine.Status() }
func (r *mlRunner) Pause()               { r.engine.Pause() }
func (r *mlRunner) Resume()              { r.engine.Resume() }
func (r *mlRunner) LogsChan() chan string { return r.engine.LogsChan() }

// Trigger requests an immediate training run, ignoring the 24-hour interval.
// If training is already in progress the request is silently dropped (the
// current run covers the need).
func (r *mlRunner) Trigger() {
	select {
	case r.trigger <- struct{}{}:
	default:
	}
}

// run is the background scheduler goroutine. It checks whether training is
// due immediately on startup, then re-checks every hour.
func (r *mlRunner) run(ctx context.Context) {
	r.maybeStart(ctx, true)

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.engine.Cancel()
			return
		case <-r.trigger:
			r.maybeStart(ctx, false) // forced, skip interval
		case <-ticker.C:
			r.maybeStart(ctx, true) // respect 24 h interval
		}
	}
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
