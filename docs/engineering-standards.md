# Engineering Standards

This document is the authoritative reference for engineering practices in Orbiter. It complements the quick-reference section in the root README with rationale, extended examples, and domain-specific rules for strategies, signals, ML, and the TUI layer.

---

## Table of Contents

1. [Tooling](#tooling)
2. [Code Style](#code-style)
3. [Error Handling](#error-handling)
4. [Structured Logging](#structured-logging)
5. [Import Grouping](#import-grouping)
6. [Context Propagation](#context-propagation)
7. [Time and Timezone](#time-and-timezone)
8. [Financial Arithmetic](#financial-arithmetic)
9. [Testing](#testing)
10. [Strategy Interface Contract](#strategy-interface-contract)
11. [Signal Architecture](#signal-architecture)
12. [TAA Engine Rules](#taa-engine-rules)
13. [ML Engine Rules](#ml-engine-rules)
14. [TUI Architecture](#tui-architecture)
15. [Background Goroutines](#background-goroutines)
16. [Schema Migrations](#schema-migrations)
17. [Dependency Minimisation](#dependency-minimisation)
18. [Clean Code](#clean-code)

---

## Tooling

Run before every commit:

```sh
goimports -w .          # format + fix import grouping (subsumes gofmt)
go vet ./...            # static analysis
go test -race ./...     # race detector — mandatory, not optional
```

Run periodically:

```sh
go test -race -cover ./...   # coverage report
go mod tidy                  # keep go.mod/go.sum clean
golangci-lint run            # extended linting (if installed)
```

Library code must never call `log.Fatal`, `os.Exit`, or `panic`. Return an error or degrade gracefully instead. The process boundary is `main.go` and `startup.Run` only.

---

## Code Style

### Comments

Default to writing **no comments**. Add one only when the _why_ is non-obvious: a hidden constraint, a subtle invariant, a workaround for a specific external behaviour. If removing the comment would not confuse a future reader, do not write it.

Never write:

- WHAT the code does (well-named identifiers already say that)
- Which task, PR, or caller this was added for (belongs in commit messages, rots in code)
- Multi-paragraph docstrings or multi-line comment blocks

```go
// wrong — describes what, not why
// Divide by n for population standard deviation
return math.Sqrt(variance / float64(len(values)))

// correct — explains a decision that surprises readers
// Population std dev (÷ n), not sample (÷ n-1): the feature pipeline normalises
// full population windows, not random samples drawn from a larger distribution.
return math.Sqrt(variance / float64(len(values)))
```

### No Premature Abstraction

Three similar lines is better than a wrong abstraction. A bug fix does not need surrounding cleanup. A one-shot operation does not need a helper. Design for the code in front of you, not for hypothetical future requirements.

### No Backwards-Compatibility Shims

Delete unused code. Do not rename to `_old`, add `// removed` comments, or re-export types for callers that no longer exist. Git history preserves everything.

---

## Error Handling

Always wrap with `%w` to preserve the chain:

```go
// correct
return fmt.Errorf("taa.Evaluate: list holdings: %w", err)

// wrong — breaks errors.Is / errors.As
return fmt.Errorf("taa.Evaluate: list holdings: %v", err)
```

Include enough context in the message for the stack to be reconstructed without a debugger. The idiomatic prefix is `package.Function: noun verb: %w`.

Never discard a returned error with `_` except in explicitly documented defer/close patterns:

```go
defer db.Close()  // acceptable: error on close is unrecoverable at this point
_ = rows.Close()  // acceptable only if error is genuinely irrelevant and documented
```

Do not add error handling or validation for scenarios that cannot happen. Trust internal invariants and framework guarantees. Only validate at system boundaries (user input, external APIs, database rows).

---

## Structured Logging

Use `log/slog` exclusively. Pass data as key-value attributes — never pre-format with `fmt.Sprintf`:

```go
// correct
slog.Error("walk-forward CV failed", "error", err)
slog.Info("rebalance signal dispatched", "symbol", h.Symbol, "conviction", conviction)

// wrong — loses machine-parseable structure
slog.Error(fmt.Sprintf("CV failed: %v", err))
```

Log levels:

| Level | When |
|---|---|
| `Debug` | Per-candle or per-tick detail; always gated, never unconditional in hot paths |
| `Info` | Significant lifecycle events (engine started, fold completed, signal dispatched) |
| `Warn` | Recoverable anomalies (PMC unavailable, TAA evaluation skipped) |
| `Error` | Failures that affect correctness and require investigation |

Never use `slog.Info` unconditionally inside `OnCandle` or `OnWarmUpCandle`. These are called on every candle across all active instruments — even a single `Info` per call generates thousands of log lines per day.

---

## Import Grouping

Three blocks separated by blank lines, enforced by `goimports`:

```go
import (
    "context"      // 1. standard library
    "fmt"
    "time"

    "github.com/shopspring/decimal"  // 2. third-party

    "github.com/tgragnato/orbiter/internal/portfolio"  // 3. internal
    "github.com/tgragnato/orbiter/pkg/ohlc"
)
```

Never mix blocks. `goimports -w .` fixes grouping automatically — run it.

---

## Context Propagation

Pass `ctx context.Context` as the first argument to every function that performs I/O, executes a database query, or may block:

```go
// correct
func (r *Repository) ListHoldings(ctx context.Context) ([]Holding, error)

// wrong — caller cannot cancel or set a deadline
func (r *Repository) ListHoldings() ([]Holding, error)
```

Never store a `Context` in a struct field. Pass it through the call chain.

When starting a long-running goroutine that should stop when the parent is done, derive a child context with `context.WithCancel` so shutdown is clean:

```go
ctx, cancel := context.WithCancel(parentCtx)
defer cancel()  // cancels child context when this scope exits
go runner.run(ctx)
```

---

## Time and Timezone

**All business logic uses UTC exclusively.**

This applies to: strategies, domain models, repositories, indicators, the TAA engine, and the ML engine.

Rules:

- `time.Now()` must always be followed by `.UTC()` in business logic: `time.Now().UTC()`
- Never call `t.Local()` or `.In(loc)` outside the TUI layer
- Never call `time.LoadLocation` in business logic; parse external timestamps with a fixed offset, then immediately normalise: `t.UTC()`
- Embed the Go timezone database in `main.go` so TUI display works in containers: `import _ "time/tzdata"`

**Strategies must support overnight positions.** They must not filter signals based on time-of-day or timezone. Historically, such filters were responsible for missed signals and subtle UTC-vs-local bugs. Time-based filtering is a portfolio-level concern, not a strategy concern.

```go
// wrong — UTC hour filter in strategy; violates overnight support
if closedCandle.End.Hour() < 10 || closedCandle.End.Hour() > 20 {
    return
}

// correct — no time-of-day filtering in strategies
func (s *StochRSI) OnCandle(closedCandle *ohlc.OHLC) {
    s.feedIndicator(closedCandle)
    // evaluate signal purely on indicator values
}
```

---

## Financial Arithmetic

Use `github.com/shopspring/decimal` for all financial values where precision matters: prices, NAV, PMC, PnL, fee amounts.

Use `float64` only where approximate values are acceptable and precision loss is documented: conviction scores `[-1.0, +1.0]`, ML feature vectors, indicator intermediates, percentage returns.

```go
// correct — position value in domain model
MarketValue() decimal.Decimal { return h.Quantity.Mul(h.MarketPrice) }

// correct — ML conviction score, precision loss acceptable
conviction := math.Tanh(forest.Predict(features) / scale)

// wrong — financial comparison using float64
if marketPrice <= pmc {  // only correct when both are float64 by design (TAA engine)
```

The TAA engine intentionally uses `float64` for market prices and PMC because its inputs come from an approximation layer (Yahoo Finance) and the precision of float64 is adequate for the threshold comparisons it performs. Document such exceptions explicitly.

---

## Testing

### Parallelism

Call `t.Parallel()` as the **first statement** in every test function, with one exception: tests that mutate package-level variables cannot safely run in parallel.

```go
func TestZScore(t *testing.T) {
    t.Parallel()
    // ...
}
```

Tests that override package-level factory variables (as in `internal/startup`) must not call `t.Parallel()`. They must use `t.Cleanup` to restore the original value:

```go
func TestRunBootstrapError(t *testing.T) {
    // no t.Parallel() — mutates package-level openPostgresFn
    orig := openPostgresFn
    t.Cleanup(func() { openPostgresFn = orig })
    openPostgresFn = func(...) (*sql.DB, error) { return nil, errors.New("boom") }
    // ...
}
```

### Coverage

Maintain ≥ 80% statement coverage across all packages. Check with:

```sh
go test -race -cover ./...
```

### Diagnostic Output

Use `t.Log` / `t.Logf`, never `fmt.Print` / `fmt.Printf`. The former appears only on test failure; the latter always appears.

### Table-Driven Tests

Use table-driven tests when the same logic is exercised across multiple input variants:

```go
tests := []struct {
    name    string
    input   float64
    want    float64
}{
    {"below lower", 10, 1.0},
    {"at mid", 50, 0.0},
    {"above upper", 90, -1.0},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        t.Parallel()
        // ...
    })
}
```

### SQL Mocks

Use `DATA-DOG/go-sqlmock` to verify that code issues the correct SQL with the correct arguments. Add one mock expectation for each migration version in tests that call `RunMigrations` — otherwise the test fails when a new migration is added.

---

## Strategy Interface Contract

Every strategy must implement `internal/strategy.Strategy`:

```go
type Strategy interface {
    GetCandleDuration() time.Duration
    OnWarmUpCandle(closedCandle *ohlc.OHLC)
    OnCandle(closedCandle *ohlc.OHLC)
    OnTick(tick *tick.Tick)
}
```

Strategies that participate in the ML feature pipeline must additionally implement `ScoredStrategy`:

```go
type ScoredStrategy interface {
    Strategy
    Score(candles []*ohlc.OHLC) float64
}
```

### Score() Contract

- Return value is in `[-1.0, +1.0]`; positive means bullish conviction, negative means bearish
- `Score()` **reads** indicator state populated by the most recent `OnCandle` call; it must **not mutate** any state
- Return `0.0` when the strategy has insufficient data to produce a conviction (warm-up not complete, indicator error)
- `Score()` is called by the ML engine between candles, not during; it must be fast (no I/O)

```go
// correct — reads RSI state, does not modify it
func (d *RSI) Score(_ []*ohlc.OHLC) float64 {
    rsiVal, err := d.getRSIValues()
    if err != nil {
        return 0
    }
    // linear mapping: oversold → +1.0, overbought → -1.0
    const mid = 50.0
    switch {
    case rsiVal <= lowerThreshold:
        return 1.0
    case rsiVal >= upperThreshold:
        return -1.0
    case rsiVal < mid:
        return (mid - rsiVal) / (mid - lowerThreshold)
    default:
        return -(rsiVal - mid) / (upperThreshold - mid)
    }
}
```

### OnCandle vs OnTick

- `OnCandle` receives a fully closed OHLC bar. Feed indicators here. Evaluate bar-based signals here.
- `OnTick` receives live price updates within an open bar. Use for intrabar breakout entry only.
- `OnWarmUpCandle` receives historical bars during indicator warm-up. Feed indicators; do not emit signals.

**Never** call `feedIndicator` more than once per candle (double-feed corrupts circular buffers and SMAs).

---

## Signal Architecture

### Dispatcher / ReadModel Separation

Producers (strategies, TAA engine) call `Dispatcher.Dispatch(msg)`. Consumers (TUI) read via `ReadModel.Pending()`. They share a `MemoryDispatcher` through the `signal.Runtime` struct.

```
signal.Runtime
  └─ MemoryDispatcher  ←  strategies / TAA engine (write)
  └─ ReadModel         →  TUI Tab 2 (read, non-blocking)
```

### Signal Types

| Type | Emitted by | Meaning |
|---|---|---|
| `BUY` | Strategies | Open a new position |
| `SELL` | Strategies | Close an open position |
| `CANCEL_ORDER` | Strategies | Cancel a pending order |
| `REBALANCE` | TAA engine | Adjust satellite allocation |
| `CORE_PMC_FLOOR_ALERT` | TAA engine | Core holding at or below purchase cost |

Never repurpose an existing signal type for a different semantic. Add a new `Type` constant and constructor function.

### Constructors

Always use the `New*` constructors in `internal/signal/signal.go`. Never build a `Message` struct literal directly in business logic — constructors centralise field assignment and `Summary` formatting.

---

## TAA Engine Rules

The TAA engine enforces two portfolio-level constraints before emitting any signal.

### Core PMC Floor

Never emit `SELL` or `REBALANCE` for a Core holding when `MarketPrice ≤ PMC` (purchase cost). Instead, emit `CORE_PMC_FLOOR_ALERT`. This prevents realising a loss on long-term Core positions during drawdowns.

If PMC data is unavailable for a symbol, skip the floor check silently (do not block, do not error).

### Satellite Friction Gate

Only emit `REBALANCE` for a Satellite holding when the expected alpha exceeds the combined transaction cost:

```
friction = effectiveFeeRate × (1 + TaxRate) + Buffer
```

where:

```
effectiveFeeRate = min(BrokerFeePercent, MaxBrokerFeeEUR / positionValue)
```

The fee cap (currently €18.90 at 0.19%) reduces friction for large positions. Use `Quantity × MarketPrice` as position value; fall back to the uncapped `BrokerFeePercent` when quantity is zero.

Default production config:

| Parameter | Value | Rationale |
|---|---|---|
| `TaxRate` | 0.26 | Italian capital gains tax (26%) |
| `BrokerFeePercent` | 0.0019 | 0.19% per trade |
| `MaxBrokerFeeEUR` | 18.90 | Broker fee cap in EUR |
| `Buffer` | 0.01 | 1% additional safety margin |
| `RebalanceThreshold` | 0.05 | Minimum 5% allocation drift |

---

## ML Engine Rules

### Pure Go Only

The ML engine (`internal/ml`) must remain dependency-free. No cgo, no external ML libraries, no CGO-linked BLAS. Reproducibility and portability matter more than raw speed for the sizes of data this engine processes.

### Reproducibility

Use the built-in LCG (`lcg`) seeded by tree index for all random operations (bootstrap sampling, feature mask selection). This ensures identical forests for identical inputs — critical for debugging and regression testing.

### Feature Pipeline

Raw prices must never be fed directly to the model. All features must be stationary (scale-invariant):

| Feature | Why |
|---|---|
| Normalised RSI | Bounded `[-1, +1]`, mean-reverting proxy |
| Z-Score (20-period) | Removes level; measures deviation in standard-deviation units |
| Percentage return | Removes price scale |
| Relative distance to SMA | Scale-invariant momentum proxy |

Features are computed in `internal/portfolio/features`. Use population standard deviation (÷ n), not sample (÷ n−1), for Z-Score normalisation — the window is a full population, not a random sample.

### Walk-Forward Cross-Validation

Training uses purged walk-forward CV to prevent temporal leakage:

```
[──── train ────][embargo][── test ──][───── future (unseen) ─────]
  T samples        E        S              slide by S each fold
```

The embargo gap prevents the model from learning on data whose labels overlap with training labels through lookahead. `Embargo` must be ≥ the maximum prediction horizon.

### Conviction Score

The forest's raw prediction is calibrated to `[-1.0, +1.0]` via `math.Tanh`:

```go
conviction := math.Tanh(forest.Predict(features) / predictionScale)
```

`predictionScale` is the standard deviation of test-set predictions across all folds, computed at training time. This ensures `Tanh` operates in a well-scaled region.

### Checkpoint Store

Persist trained forests to PostgreSQL (`ml_model_checkpoints`) using gob serialisation to BYTEA. Keep at most 5 intermediate checkpoints per model name (prune oldest). The currently active model is marked with `is_active = true`; demote all others before promoting a new one. Use exported mirror types (`nodeData`, `treeData`, `forestData`) for gob compatibility — gob cannot encode unexported fields.

---

## TUI Architecture

### Bubble Tea Model Rules

- Every `Update` method must return `(tea.Model, tea.Cmd)` — never mutate state directly outside of a returned model copy
- Use non-blocking `tea.Cmd` functions for I/O; never block inside `Update`
- Use `tea.Tick` for periodic refresh; never use `time.Sleep` inside a model

### MLEngine Interface

The TUI communicates with the ML engine through `signals.MLEngine`:

```go
type MLEngine interface {
    Status() int32       // StatusIdle / Running / Paused / Done
    Pause()
    Resume()
    Trigger()            // request immediate run, bypasses 24-hour interval
    LogsChan() chan string
}
```

`Trigger()` is non-blocking. The scheduler goroutine consumes it asynchronously. If the engine is already running, the trigger is silently dropped.

### Keybindings (Tab 2)

| Key | Action |
|---|---|
| `p` | Pause training if running; resume if paused |
| `r` | Trigger a new training run (no-op if already running) |
| `tab` / `l` | Switch to Tab 1 |

### Log Ring Buffer

ML training logs are kept in a ring buffer of 20 lines (`mlLogMaxLines`). Use `appendRing(buf, line, max)` to add entries — it trims from the front, keeping the most recent lines.

Drain the ML log channel non-blocking via `drainMLLogsCmd()`, which reads at most one line per tick. This keeps UI latency low regardless of training throughput.

---

## Background Goroutines

### Lifecycle

Every background goroutine must:

1. Accept `ctx context.Context` and exit when `ctx.Done()` closes
2. Use `time.NewTicker` (not `time.Sleep`) for periodic work — tickers are stoppable
3. Call `defer ticker.Stop()` immediately after creating a ticker

```go
func (r *mlRunner) run(ctx context.Context) {
    ticker := time.NewTicker(time.Hour)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            r.maybeStart(ctx, true)
        }
    }
}
```

### Startup Context

`startup.Run` derives a child context with `context.WithCancel`. The `defer cancel()` ensures all background goroutines (TAA evaluator, ML scheduler) stop when the TUI exits — even on the happy path:

```go
ctx, cancel := context.WithCancel(ctx)
defer cancel()
```

### Goroutine Ownership

The goroutine that starts a background worker owns it. If the owner exits, it must cancel the worker's context. Never start a goroutine that outlives its owner's context without explicit design documentation.

---

## Schema Migrations

### Versioning

Migrations are numbered sequentially (`version BIGINT`). Never reuse a version number. Never modify an applied migration — add a new one instead.

Current versions:

| Version | Name |
|---|---|
| 1 | `create_app_settings` |
| 2 | `create_holdings_and_allocation_type` |
| 3 | `create_corporate_actions_and_accounting_tables` |
| 4 | `create_twr_analytics_tables` |
| 5 | `create_ml_model_checkpoints` |

### Tests

Every test that calls `RunMigrations` or `Bootstrap` must include a mock expectation for **every** migration version. When adding a migration, update all affected test mock sequences. The CI will fail otherwise because `go-sqlmock` uses strict expectation matching.

---

## Dependency Minimisation

Prefer the Go standard library. A new dependency is justified only when it provides functionality that is genuinely impractical to implement correctly in-house.

| Dependency | Justification |
|---|---|
| `charmbracelet/bubbletea`, `bubbles`, `lipgloss` | Terminal UI — no stdlib equivalent |
| `jackc/pgx/v5` | PostgreSQL driver — `database/sql` requires a driver |
| `shopspring/decimal` | Exact decimal arithmetic — `float64` unsuitable for financial values |
| `markcheno/go-talib` | TA-Lib indicator algorithms (RSI, ADX, Stoch, StochRSI) |
| `DATA-DOG/go-sqlmock` | SQL mock for unit tests — test-only |

Before adding a dependency:
1. Confirm the standard library does not cover the use case
2. Confirm the package is actively maintained
3. Confirm the license is compatible with AGPL-3.0
4. Confirm it adds no CGO requirement unless absolutely unavoidable

---

## Clean Code

- Dead code must not exist. If functionality is removed, delete it — Git preserves history.
- Commented-out code blocks are not acceptable.
- No `TODO` or `FIXME` comments in committed code; use issues instead.
- Unused variables assigned to `_` must include a comment if the discard is non-obvious.
- Exported types, functions, and constants must have a doc comment. Unexported ones do not.
