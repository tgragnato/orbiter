package startup

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/tgragnato/orbiter/internal/backup"
	"github.com/tgragnato/orbiter/internal/configuration"
	"github.com/tgragnato/orbiter/internal/ml"
	"github.com/tgragnato/orbiter/internal/portfolio"
	"github.com/tgragnato/orbiter/internal/portfolio/analytics"
	"github.com/tgragnato/orbiter/internal/portfolio/data"
	"github.com/tgragnato/orbiter/internal/portfolio/featurizer"
	"github.com/tgragnato/orbiter/internal/portfolio/feed"
	"github.com/tgragnato/orbiter/internal/portfolio/fx"
	"github.com/tgragnato/orbiter/internal/signal"
	"github.com/tgragnato/orbiter/internal/signal/taa"
	signals "github.com/tgragnato/orbiter/internal/tui"
)

type config struct {
	dsn string
}

var (
	openPostgresFn = openPostgres
	bootstrapFn    = configuration.Bootstrap
	newSignalRTFn  = signal.NewRuntime
	newStoreFn     = func(db *sql.DB) portfolio.HoldingsStore { return portfolio.NewPostgresStore(db) }
	newRootModelFn = func(
		store portfolio.HoldingsStore,
		readModel signal.ReadModel,
		mlEngine signals.MLEngine,
		txStore portfolio.TransactionStore,
		configSvc signals.SettingsService,
		logCh signals.LogChannel,
		twrEngine *analytics.TWREngine,
		baseCurrency string,
	) tea.Model {
		return signals.NewRootModelWithMetrics(store, readModel, "MAIN", mlEngine, txStore, configSvc, logCh, twrEngine, baseCurrency)
	}
	newProgramFn = func(model tea.Model, options ...tea.ProgramOption) programRunner {
		return tea.NewProgram(model, options...)
	}
)

type programRunner interface {
	Run() (tea.Model, error)
}

// Run dispatches to backup/restore subcommands or starts the TUI.
func Run(ctx context.Context, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "backup":
			return backup.RunBackup(ctx, args[1:], os.Getenv)
		case "restore":
			return backup.RunRestore(ctx, args[1:], os.Getenv)
		}
	}
	return runTUI(ctx, args)
}

func runTUI(ctx context.Context, args []string) error {
	conf, err := parseConfig(args, os.Getenv)
	if err != nil {
		return err
	}

	db, err := openPostgresFn(ctx, conf.dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	configSvc, err := bootstrapFn(ctx, db)
	if err != nil {
		return fmt.Errorf("configuration bootstrap failed: %w", err)
	}

	// Derive a child context so background goroutines stop when the TUI exits.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	signalRuntime := newSignalRTFn()
	store := newStoreFn(db)

	// *portfolio.PostgresStore implements both TransactionStore and feed.PriceStore.
	// In tests the injected fake may not, so we guard with type assertions.
	var txStore portfolio.TransactionStore
	if ts, ok := store.(portfolio.TransactionStore); ok {
		txStore = ts
	}
	var priceStore feed.PriceStore
	if ps, ok := store.(feed.PriceStore); ok {
		priceStore = ps
	}

	// Redirect slog to the TUI log tab before any goroutine starts.
	logCh := signals.NewLogChannel()
	slog.SetDefault(slog.New(signals.NewTUIHandler(logCh)))

	// ML engine with 24-hour auto-scheduling, checkpoint persistence, and
	// per-symbol conviction scoring for the TAA engine.
	//
	// The raw Yahoo provider is wrapped with a DB-backed candle cache so that
	// historical EOD data is fetched from Yahoo only once per symbol. Subsequent
	// featurizer runs (training + inference) query the local eod_candles table
	// and only request the delta (new days since last cache update) from Yahoo.
	yahooProvider := data.NewYahooProvider(&http.Client{Timeout: 30 * time.Second})
	var candleProvider data.DataProvider = yahooProvider
	if cs, ok := store.(data.CandleStorer); ok {
		candleProvider = data.NewCachingProvider(yahooProvider, cs)
	}
	ckpt := ml.NewCheckpoint(db)
	runner := newMLRunner(ml.NewEngine(), func() []ml.Sample {
		samples, err := featurizer.ExtractMLSamples(ctx, store, candleProvider)
		if err != nil {
			slog.Warn("ml: sample extraction failed", "error", err)
			return nil
		}
		return samples
	}, ml.WalkForwardConfig{
		TrainSize:        1250,
		TestSize:         60,
		Embargo:          10,
		LabelHorizon:     5,
		NTrees:           50,
		FeaturesPerSplit: 12,
		MaxDepth:         5,
		MinSamples:       10,
	}, ckpt, func(ctx context.Context) (map[string]ml.Sample, error) {
		return featurizer.CurrentSamples(ctx, store, candleProvider)
	})
	go runner.run(ctx)

	// Bridge ML engine raw log lines into the shared TUI log channel so they
	// appear in the Logs tab. This runs independently of any TUI component and
	// owns the drain loop for the lifetime of the process.
	go func() {
		ch := runner.LogsChan()
		for {
			select {
			case <-ctx.Done():
				return
			case line, ok := <-ch:
				if !ok {
					return
				}
				entry := signals.LogEntry{
					Time:    time.Now(),
					Level:   slog.LevelInfo,
					Message: line,
					Attrs:   []slog.Attr{slog.String("source", "ml-engine")},
				}
				select {
				case logCh <- entry:
				default:
				}
			}
		}
	}()

	// TAA engine: 0.19 % broker fee capped at €18.90, evaluated every 24 h.
	taaEngine := taa.NewEngine(
		store,
		taa.NullPMCReader{},
		runner,
		runner,
		signalRuntime.Dispatcher,
		taa.Config{
			TaxRate:          0.26,
			BrokerFeePercent: 0.0019,
			MaxBrokerFeeEUR:  18.90,
			Buffer:           0.01,
		},
	)

	// Block until the conviction map is populated from the DB checkpoint before
	// the first TAA evaluation. Without this barrier, Evaluate races with
	// seedConvictionScores and reads an empty map — all satellite signals are
	// suppressed because conviction=0 never exceeds the friction gate.
	<-runner.convictionReady

	go func() {
		if err := taaEngine.Evaluate(ctx); err != nil {
			slog.Warn("taa initial evaluation failed", "error", err)
		}
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := taaEngine.Evaluate(ctx); err != nil {
					slog.Warn("taa periodic evaluation failed", "error", err)
				}
			}
		}
	}()

	analyticsRepo := analytics.NewPostgresRepository(db)
	twrEngine := analytics.NewTWREngine(analyticsRepo)

	// Enable live cash-flow recording so TWR stays correct within the current
	// session after AddTransaction, without waiting for the next startup backfill.
	if ps, ok := store.(*portfolio.PostgresStore); ok {
		ps.WithPortfolioID("MAIN")
	}

	// FX engine: resolves historical and live exchange rates via Yahoo Finance.
	// The provider lives in the data package (shared Yahoo client — no duplication).
	fxStore := fx.NewPostgresStore(db)
	fxProvider := data.NewYahooFXProvider(&http.Client{Timeout: 30 * time.Second})
	fxSvc := fx.NewService(fxProvider, fxStore)

	// Portfolio base currency — falls back to "EUR" if not configured.
	baseCurrency := "EUR"
	if configSvc != nil {
		if bc, err := configSvc.GetBaseCurrency(ctx); err == nil && bc != "" {
			baseCurrency = bc
		} else if err != nil {
			slog.Warn("startup: could not read base currency, defaulting to EUR", "error", err)
		}
	}

	// Price feed: refresh EOD quotes every 30 minutes and sync dividend income when
	// the store supports the DividendSyncer interface (no-op otherwise).
	// After each refresh a NAV snapshot is recorded so the TWR chart accumulates
	// data points over time. FX service converts multi-currency NAVs to base currency.
	if priceStore != nil {
		var priceFeed *feed.Updater
		if ds, ok := store.(feed.DividendSyncer); ok {
			priceFeed = feed.NewWithDividendSync(priceStore, ds, candleProvider, 30*time.Minute)
		} else {
			priceFeed = feed.New(priceStore, candleProvider, 30*time.Minute)
		}
		priceFeed.WithNAVSnapshot(store, twrEngine, "MAIN")
		priceFeed.WithFXService(fxSvc, baseCurrency)
		if bf, ok := store.(feed.NAVBackfiller); ok {
			priceFeed.WithNAVBackfill(bf)
			priceFeed.WithCashFlowRecorder(twrEngine)
		}
		if sp, ok := store.(feed.SplitPersister); ok {
			priceFeed.WithSplitPersister(sp)
		}
		if ws, ok := store.(feed.WatchlistPriceStore); ok {
			priceFeed.WithWatchlistUpdater(ws)
		}
		go priceFeed.Run(ctx)
	}

	// Wrap configSvc in an interface to avoid a non-nil interface holding a nil pointer.
	var settingsSvc signals.SettingsService
	if configSvc != nil {
		settingsSvc = configSvc
	}
	rootModel := newRootModelFn(store, signalRuntime.ReadModel, runner, txStore, settingsSvc, logCh, twrEngine, baseCurrency)
	program := newProgramFn(rootModel, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("tui runtime failed: %w", err)
	}

	return nil
}

func parseConfig(args []string, lookupEnv func(string) string) (config, error) {
	var conf config

	fs := flag.NewFlagSet("orbiter", flag.ContinueOnError)
	fs.StringVar(&conf.dsn, "dsn", "", "PostgreSQL DSN")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if len(fs.Args()) > 0 {
		return config{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}

	conf.dsn = strings.TrimSpace(conf.dsn)
	if conf.dsn == "" {
		conf.dsn = strings.TrimSpace(lookupEnv("DATABASE_URL"))
	}
	if conf.dsn == "" {
		return config{}, errors.New("missing PostgreSQL DSN: provide --dsn or DATABASE_URL")
	}

	return conf, nil
}

func openPostgres(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database with DSN %q: %w", dsn, err)
	}
	return db, nil
}
