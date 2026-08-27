# 🛰️ Orbiter - Core-Satellite Portfolio Manager

A Go application for managing a Core-Satellite portfolio workflow with PostgreSQL-backed configuration, a Bubble Tea terminal interface, and manual execution signals.

This repository has been refactored away from direct broker automation into a keyboard-driven portfolio and signal workstation:

- Tab 1: Unified holdings management (Core + Satellite in one table)
- Tab 2: Non-blocking TAA signal queue viewer with ML confidence scores
- Tab 3: Live settings editor backed by PostgreSQL
- Tab 4: Structured log viewer (slog stream)
- Tab 5: Transaction ledger with add/edit support
- Tab 6: Portfolio analytics (cumulative TWR, drawdown, and rolling Sortino charts)
- Runtime configuration: database-driven, loaded from PostgreSQL

## Project Overview

Orbiter focuses on a Core-Satellite portfolio process where tactical decisions become signals in the terminal UI for manual execution.

The runtime architecture is:

1. Start with a PostgreSQL DSN only.
2. Run schema migrations and bootstrap settings from the database.
3. Launch a Bubble Tea root model composed of six tabs:
   1. Holdings — unified Core + Satellite view
   2. Signals — TAA signal queue with ML confidence scores
   3. Settings — live PostgreSQL-backed configuration editor
   4. Logs — structured slog stream
   5. Transactions — ledger with add/edit support
   6. Analytics — cumulative TWR, drawdown from peak, and rolling Sortino charts

This keeps startup deterministic and centralizes operational configuration in PostgreSQL instead of CLI flag sprawl.

## Startup Contract (Strict DSN-Only)

The runtime startup contract is **single-parameter only**:

```sh
go run . --dsn postgres://postgres:postgres@localhost:5432/orbiter?sslmode=disable
```

or via environment variable:

```sh
DATABASE_URL=postgres://postgres:postgres@localhost:5432/orbiter?sslmode=disable go run .
```

Only PostgreSQL DSN is accepted at startup. All application settings are loaded from PostgreSQL (`app_settings`) during bootstrap.

Bootstrap sequence:

1. Open PostgreSQL using `--dsn` or `DATABASE_URL`.
2. Run schema migrations (`schema_migrations`, `app_settings`, `holdings`).
3. Seed default application settings if `app_settings` is empty.
4. Validate required settings through the configuration service.

Settings seeded and validated include:

- Yahoo Finance API credentials (`credentials.yahoo`)
- Portfolio base currency (`portfolio.base_currency`, default EUR)

## Key Features

### Unified Holdings TUI (Tab 1)

- Single-table view for both Core and Satellite assets
- Allocation badges rendered in `[CORE]` / `[SAT]` style chips
- NAV summary bar with total, core %, satellite %, TWR %, and realized PnL
- Instant allocation toggle with `t` key
- Non-blocking periodic refresh

### Signals TUI (Tab 2)

- Non-blocking TAA signal queue display
- Polls queued signal read model without blocking the rest of the UI
- Shows ML confidence scores from the random forest engine
- Designed for manual rebalance execution workflows

### Settings TUI (Tab 3)

- Live editor for all PostgreSQL-backed `app_settings`
- Changes take effect without restarting the application

### Logs TUI (Tab 4)

- Structured `slog` stream redirected from the default logger
- Captures output from background goroutines (TAA, ML, price feed)

### Transactions TUI (Tab 5)

- Full transaction ledger with keyboard-driven add and edit forms
- Mutations in this tab trigger an automatic refresh of the holdings tab

### Analytics TUI (Tab 6)

- ASCII charts rendered with `asciigraph`
- **Cumulative TWR (%)**: geometric chain of all sub-period returns since inception
- **Drawdown from Peak (%)**: rolling maximum drawdown at each NAV snapshot
- **Rolling Sortino Ratio**: accumulated over the full return history (MAR = 0, downside deviation only)
- Summary stat bar: total TWR, CAGR, max drawdown, and current Sortino score
- `r` key reloads data from PostgreSQL without restarting the application

### ML Engine

- Random forest regressor trained on 36-feature EOD sample vectors
- Features in six groups: batch go-talib indicators (0–12), streaming `ScoredStrategy` conviction scores (13–22), incremental `pkg/indicator` outputs (23–25), relative strength vs the portfolio benchmark plus medium-term momentum (26–29), the primary trend regime from SMA50/SMA200 distance and SMA50 slope (30–32), and the volatility regime from normalised ATR and the σ20/σ60 compression ratio (33–35)
- Walk-forward cross-validation with configurable train/test/embargo windows; all folds merged into a single ensemble via `MergeForests`
- 24-hour auto-scheduling; best forest persisted to PostgreSQL and conviction scores seeded on restart

### Backup and Restore

- `backup` subcommand: exports all transactions to a versioned JSON file
- `restore` subcommand: re-inserts transactions from a JSON backup via `AddTransaction` (additive — truncate tables first for a clean restore)

### Analytics and Accounting

- Weighted average cost accounting (`PMC`)
- Time-Weighted Return (TWR) engine with sub-period geometric chaining
- Corporate actions handling: splits and dividends
- Realized PnL tracking with tax-lot links

### Single Parameter Startup

- Strict `--dsn` or `DATABASE_URL` startup contract
- No additional runtime parameters required to launch the portfolio UI

## Architecture

Runtime data flow:

```
main.go
  └─ startup.Run
       ├─ backup / restore subcommands (JSON transaction export / import)
       └─ runTUI
            ├─ parseConfig  (--dsn / DATABASE_URL)
            ├─ openPostgres
            ├─ configuration.Bootstrap
            │    ├─ schema migrations  (app_settings, holdings, …)
            │    └─ seed default settings
            │
            ├─ signal.NewRuntime  ──► SignalRuntime { Dispatcher, ReadModel }
            │
            ├─ portfolio.NewPostgresStore  (HoldingsStore + TransactionStore + PriceStore)
            │
            ├─ [goroutine] mlRunner  (1h auto-schedule)
            │    ├─ featurizer.ExtractMLSamples  ──► portfolio store + Yahoo data
            │    │    ├─ go-talib batch indicators       (features 0–12)
            │    │    └─ strategy.ScoredStrategy scores  (features 13–22)
            │    └─ ml.Engine  (Random Forest, WalkForward CV)
            │
            ├─ [goroutine] taa.Engine  (1h ticker)
            │    ├─ reads holdings from store
            │    ├─ applies conviction + PMC reader
            │    └─ dispatches signals ──► signal.Dispatcher
            │
            ├─ [goroutine] feed.Updater  (30 min ticker)
            │    ├─ data.YahooProvider  (EOD OHLC, UCITS/EUR)
            │    └─ DividendSyncer  (syncs income when store supports it)
            │
            └─ Bubble Tea RootModel  (alt-screen)
                 ├─ Tab 1: Holdings     (HoldingsStore, TransactionStore)
                 ├─ Tab 2: Signals      (signal.ReadModel, ml.Engine)
                 ├─ Tab 3: Settings     (configuration.Service)
                 ├─ Tab 4: Logs         (slog → TUIHandler → LogChannel)
                 ├─ Tab 5: Transactions (TransactionEditor)
                 └─ Tab 6: Analytics    (analytics.TWREngine → TWR / drawdown / Sortino charts)
```

## Getting Started

### 1) Run Migrations and Bootstrap

Bootstrap runs migrations and validates required settings automatically during startup.

```sh
go run . --dsn postgres://postgres:postgres@localhost:5432/orbiter?sslmode=disable
```

or:

```sh
DATABASE_URL=postgres://postgres:postgres@localhost:5432/orbiter?sslmode=disable go run .
```

## Development

### Prerequisites

- Go 1.27+
- PostgreSQL

### Common Tasks

Run the commands directly:

```sh
go build ./...                    # compile all packages
go test -race -cover ./...        # run tests with race detector and coverage
go vet ./...                      # run vet checks
gofmt -w $(find . -name '*.go')   # format all Go sources
golangci-lint run                 # run linter (if installed)
go mod tidy                       # clean go.mod/go.sum
```

### Documentation

- [/docs/engineering-standards.md](/docs/engineering-standards.md)
- [/docs/ml-engine.md](/docs/ml-engine.md)
- [/docs/ai-guidelines.md](/docs/ai-guidelines.md)

## Disclaimer

The developers are not liable for losses arising from portfolio or trading decisions. Use this software at your own risk.
