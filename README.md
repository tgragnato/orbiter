# 🛰️ Orbiter - Core-Satellite Portfolio Manager

A Go application for managing a Core-Satellite portfolio workflow with PostgreSQL-backed configuration, a Bubble Tea terminal interface, and manual execution signals.

This repository has been refactored away from direct broker automation into a keyboard-driven portfolio and signal workstation:

- Tab 1: Unified holdings management (Core + Satellite in one table)
- Tab 2: Non-blocking TAA signal queue viewer
- Runtime configuration: database-driven, loaded from PostgreSQL

## Project Overview

Orbiter focuses on a Core-Satellite portfolio process where tactical decisions become signals in the terminal UI for manual execution.

The runtime architecture is:

1. Start with a PostgreSQL DSN only.
2. Run schema migrations and bootstrap settings from the database.
3. Launch a Bubble Tea root model composed of:
1. Tab 1 Unified Holdings
2. Tab 2 Signals Queue

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

- Cost basis method (PMC default; FIFO/LIFO supported)
- Data provider preferences (Yahoo default, EUR)
- TAA parameters
- Core/satellite target ratios
- TUI preferences
- Yahoo credential slot

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
- Designed for manual rebalance execution workflows

### Analytics and Accounting

- Configurable cost basis policy in PostgreSQL (`PMC`, `FIFO`, `LIFO`)
- Time-Weighted Return (TWR) engine with sub-period geometric chaining
- Corporate actions handling: splits and dividends
- Realized PnL tracking with tax-lot links

### Single Parameter Startup

- Strict `--dsn` or `DATABASE_URL` startup contract
- No additional runtime parameters required to launch the portfolio UI

## Architecture

Runtime data flow:

```
PostgreSQL DSN -> startup.Run -> migrations/bootstrap -> root Bubble Tea program
                                                 |-> Tab 1 holdings store (DB)
                                                 |-> Tab 2 signal read model
```

### Package Map

- `main.go`: process entrypoint, forwards CLI args to startup
- `internal/startup`: strict DSN parsing, DB open, migration/bootstrap, root TUI launch
- `internal/configuration`: schema migrations and typed DB-backed settings service
- `internal/portfolio`: holdings domain model and PostgreSQL holdings store
- `internal/portfolio/accounting`: cost basis calculators (PMC, FIFO, LIFO) and realized PnL ledger
- `internal/portfolio/analytics`: TWR engine with cash flow and NAV snapshot persistence
- `internal/portfolio/corporate`: corporate actions service (splits, dividends)
- `internal/portfolio/data`: market data provider (Yahoo Finance UCITS/European EOD adapter)
- `internal/signal`: signal message model and queue read-side model
- `internal/tui/signals`: root tab model, Tab 1 holdings model, Tab 2 signals model
- `internal/trader`: strategy/trading orchestration and signal dispatch production

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

### 2) Launch the TUI

The same startup command launches the Bubble Tea UI in alt-screen mode.

Keyboard essentials:

- `tab` or `l`: next tab
- `shift+tab` or `h`: previous tab
- `t`: toggle allocation type on selected holding (Tab 1)
- `q` or `Ctrl+C`: quit

## Development

### Prerequisites

- Go 1.26+
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

## Disclaimer

The developers are not liable for losses arising from portfolio or trading decisions. Use this software at your own risk.
