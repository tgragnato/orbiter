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
	"github.com/tgragnato/orbiter/internal/portfolio/data"
	"github.com/tgragnato/orbiter/internal/portfolio/feed"
	"github.com/tgragnato/orbiter/internal/portfolio/featurizer"
	"github.com/tgragnato/orbiter/internal/signal"
	"github.com/tgragnato/orbiter/internal/signal/taa"
	signals "github.com/tgragnato/orbiter/internal/tui"
)

type config struct {
	dsn string
}

// configTargetReader adapts configuration.Service to the taa.TargetReader interface.
type configTargetReader struct{ svc *configuration.Service }

func (r *configTargetReader) GetCoreSatelliteTargets(ctx context.Context) (taa.CoreSatelliteTargets, error) {
	s, err := r.svc.GetCoreSatelliteTargets(ctx)
	if err != nil {
		return taa.CoreSatelliteTargets{}, err
	}
	return taa.CoreSatelliteTargets{CoreRatio: s.CoreRatio, SatelliteRatio: s.SatelliteRatio}, nil
}

func (r *configTargetReader) GetRebalanceThreshold(ctx context.Context) (float64, error) {
	s, err := r.svc.GetTAA(ctx)
	if err != nil {
		return 0, err
	}
	return s.RebalanceThreshold, nil
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
	) tea.Model {
		return signals.NewRootModelWithMetrics(store, readModel, "MAIN", mlEngine, txStore, configSvc, logCh)
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
	defer db.Close()

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
	yahooProvider := data.NewYahooProvider(&http.Client{Timeout: 30 * time.Second})
	ckpt := ml.NewCheckpoint(db)
	runner := newMLRunner(ml.NewEngine(), func() []ml.Sample {
		samples, err := featurizer.ExtractMLSamples(ctx, store, yahooProvider)
		if err != nil {
			slog.Warn("ml: sample extraction failed", "error", err)
			return nil
		}
		return samples
	}, ml.WalkForwardConfig{
		TrainSize:  300,
		TestSize:   60,
		Embargo:    10,
		NTrees:     50,
		MaxDepth:   5,
		MinSamples: 10,
	}, ckpt, func(ctx context.Context) (map[string]ml.Sample, error) {
		return featurizer.CurrentSamples(ctx, store, yahooProvider)
	})
	go runner.run(ctx)
	go runner.seedFromCheckpoint(ctx)

	// TAA engine: 0.19 % broker fee capped at €18.90, evaluated every 24 h.
	taaEngine := taa.NewEngine(
		store,
		taa.NullPMCReader{},
		runner,
		signalRuntime.Dispatcher,
		taa.Config{
			TaxRate:            0.26,
			BrokerFeePercent:   0.0019,
			MaxBrokerFeeEUR:    18.90,
			Buffer:             0.01,
			RebalanceThreshold: 0.05,
		},
		&configTargetReader{svc: configSvc},
	)
	go func() {
		if err := taaEngine.Evaluate(ctx); err != nil {
			slog.Warn("taa initial evaluation failed", "error", err)
		}
		ticker := time.NewTicker(24 * time.Hour)
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

	// Price feed: refresh EOD quotes every 30 minutes and sync dividend income when
	// the store supports the DividendSyncer interface (no-op otherwise).
	if priceStore != nil {
		var priceFeed *feed.Updater
		if ds, ok := store.(feed.DividendSyncer); ok {
			priceFeed = feed.NewWithDividendSync(priceStore, ds, yahooProvider, 30*time.Minute)
		} else {
			priceFeed = feed.New(priceStore, yahooProvider, 30*time.Minute)
		}
		go priceFeed.Run(ctx)
	}

	// Wrap configSvc in an interface to avoid a non-nil interface holding a nil pointer.
	var settingsSvc signals.SettingsService
	if configSvc != nil {
		settingsSvc = configSvc
	}
	rootModel := newRootModelFn(store, signalRuntime.ReadModel, runner, txStore, settingsSvc, logCh)
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
		db.Close()
		return nil, fmt.Errorf("failed to ping database with DSN %q: %w", dsn, err)
	}
	return db, nil
}
