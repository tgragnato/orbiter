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

// Sentinel errors for configuration parsing.
var (
	// ErrUnexpectedArgs is returned when unexpected positional arguments are passed.
	ErrUnexpectedArgs = errors.New("unexpected positional arguments")
	// ErrMissingDSN is returned when no PostgreSQL DSN is provided.
	ErrMissingDSN = errors.New("missing PostgreSQL DSN: provide --dsn or DATABASE_URL")
)

// Default broker configuration values.
const (
	defaultTaxRate          = 0.26
	defaultBrokerFeePercent = 0.0019
	defaultMaxBrokerFee     = 18.90
	defaultBuffer           = 0.01
)

// Default ML walk-forward configuration values.
const (
	mlTrainSize        = 1250
	mlTestSize         = 60
	mlEmbargo          = 10
	mlLabelHorizon     = 5
	mlNTrees           = 50
	mlFeaturesPerSplit = 12
	mlMaxDepth         = 5
	mlMinSamples       = 10
)

// priceFeedInterval is the interval between price feed refresh runs.
const priceFeedInterval = 30 * time.Minute

// httpClientTimeout is the timeout for the Yahoo Finance HTTP client.
const httpClientTimeout = 30 * time.Second

type config struct {
	dsn string
}

//nolint:gochecknoglobals // test injection points; replaced in tests via t.Cleanup
var (
	openPostgresFn = openPostgres
	bootstrapFn    = configuration.Bootstrap
	newSignalRTFn  = signal.NewRuntime
	newStoreFn     = func(sqlDB *sql.DB) portfolio.HoldingsStore { return portfolio.NewPostgresStore(sqlDB) }
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
		return signals.NewRootModelWithMetrics(
			store, readModel, "MAIN", mlEngine, txStore, configSvc, logCh, twrEngine, baseCurrency,
		)
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
			err := backup.RunBackup(ctx, args[1:], os.Getenv)
			if err != nil {
				return fmt.Errorf("backup: %w", err)
			}

			return nil
		case "restore":
			err := backup.RunRestore(ctx, args[1:], os.Getenv)
			if err != nil {
				return fmt.Errorf("restore: %w", err)
			}

			return nil
		}
	}

	return runTUI(ctx, args)
}

//nolint:gocognit,gocyclo,cyclop,nestif,funlen,maintidx // startup wiring; inherent complexity
func runTUI(ctx context.Context, args []string) error {
	conf, err := parseConfig(args, os.Getenv)
	if err != nil {
		return err
	}

	sqlDB, err := openPostgresFn(ctx, conf.dsn)
	if err != nil {
		return err
	}

	defer func() { _ = sqlDB.Close() }()

	configSvc, err := bootstrapFn(ctx, sqlDB)
	if err != nil {
		return fmt.Errorf("configuration bootstrap failed: %w", err)
	}

	// Derive a child context so background goroutines stop when the TUI exits.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	signalRuntime := newSignalRTFn()
	store := newStoreFn(sqlDB)

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

	// Load configuration settings (base currency, Yahoo credentials, broker params).
	var yahooAPIKey string

	baseCurrency := "EUR"
	brokerCfg := configuration.TAABrokerConfig{
		TaxRate:          defaultTaxRate,
		BrokerFeePercent: defaultBrokerFeePercent,
		MaxBrokerFee:     defaultMaxBrokerFee,
		Buffer:           defaultBuffer,
	}

	if configSvc != nil {
		creds, credErr := configSvc.GetYahooCredentials(ctx)
		if credErr == nil {
			yahooAPIKey = creds.APIKey
		} else {
			slog.Warn("startup: could not read yahoo credentials", "error", credErr)
		}

		bc, bcErr := configSvc.GetBaseCurrency(ctx)
		if bcErr == nil && bc != "" {
			baseCurrency = bc
		} else if bcErr != nil {
			slog.Warn("startup: could not read base currency, defaulting to EUR", "error", bcErr)
		}

		fetchedBrokerCfg, brokerErr := configSvc.GetBrokerConfig(ctx)
		if brokerErr == nil {
			brokerCfg = fetchedBrokerCfg
		} else {
			slog.Warn("startup: could not read broker config, using defaults", "error", brokerErr)
		}
	}

	// ML engine with 24-hour auto-scheduling, checkpoint persistence, and
	// per-symbol conviction scoring for the TAA engine.
	//
	// The raw Yahoo provider is wrapped with a DB-backed candle cache so that
	// historical EOD data is fetched from Yahoo only once per symbol. Subsequent
	// featurizer runs (training + inference) query the local eod_candles table
	// and only request the delta (new days since last cache update) from Yahoo.
	yahooProvider := data.NewYahooProvider(&http.Client{
		Timeout:       httpClientTimeout,
		Transport:     nil,
		CheckRedirect: nil,
		Jar:           nil,
	}).WithAPIKey(yahooAPIKey)

	var candleProvider data.DataProvider = yahooProvider

	if cs, ok := store.(data.CandleStorer); ok {
		candleProvider = data.NewCachingProvider(yahooProvider, cs)
	}

	ckpt := ml.NewCheckpoint(sqlDB)

	runner := newMLRunner(ml.NewEngine(), func() []ml.Sample {
		samples, sampleErr := featurizer.ExtractMLSamples(ctx, store, candleProvider)
		if sampleErr != nil {
			slog.Warn("ml: sample extraction failed", "error", sampleErr)

			return nil
		}

		return samples
	}, ml.WalkForwardConfig{
		TrainSize:        mlTrainSize,
		TestSize:         mlTestSize,
		Embargo:          mlEmbargo,
		LabelHorizon:     mlLabelHorizon,
		NTrees:           mlNTrees,
		FeaturesPerSplit: mlFeaturesPerSplit,
		MaxDepth:         mlMaxDepth,
		MinSamples:       mlMinSamples,
	}, ckpt, func(innerCtx context.Context) (map[string]ml.Sample, error) {
		return featurizer.CurrentSamples(innerCtx, store, candleProvider)
	})

	go runner.run(ctx)

	// Bridge ML engine raw log lines into the shared TUI log channel so they
	// appear in the Logs tab. This runs independently of any TUI component and
	// owns the drain loop for the lifetime of the process.
	go func() {
		logsCh := runner.LogsChan()

		for {
			select {
			case <-ctx.Done():
				return
			case line, ok := <-logsCh:
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

	// TAA engine: friction parameters are read from the DB at startup and can be
	// updated at runtime from the Settings tab via taaEngine.SetConfig.
	taaEngine := taa.NewEngine(
		store,
		taa.NullPMCReader{},
		runner,
		runner,
		signalRuntime.Dispatcher,
		taa.Config{
			TaxRate:          brokerCfg.TaxRate,
			BrokerFeePercent: brokerCfg.BrokerFeePercent,
			MaxBrokerFee:     brokerCfg.MaxBrokerFee,
			Buffer:           brokerCfg.Buffer,
			Currency:         baseCurrency,
		},
	)

	// Block until the conviction map is populated from the DB checkpoint before
	// the first TAA evaluation. Without this barrier, Evaluate races with
	// seedConvictionScores and reads an empty map — all satellite signals are
	// suppressed because conviction=0 never exceeds the friction gate.
	<-runner.convictionReady

	go func() {
		evalErr := taaEngine.Evaluate(ctx)
		if evalErr != nil {
			slog.Warn("taa initial evaluation failed", "error", evalErr)
		}

		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				periodicErr := taaEngine.Evaluate(ctx)
				if periodicErr != nil {
					slog.Warn("taa periodic evaluation failed", "error", periodicErr)
				}
			}
		}
	}()

	analyticsRepo := analytics.NewPostgresRepository(sqlDB)
	twrEngine := analytics.NewTWREngine(analyticsRepo)

	// Enable live cash-flow recording so TWR stays correct within the current
	// session after AddTransaction, without waiting for the next startup backfill.
	if ps, ok := store.(*portfolio.PostgresStore); ok {
		ps.WithPortfolioID("MAIN")
	}

	// FX engine: resolves historical and live exchange rates via Yahoo Finance.
	// The provider lives in the data package (shared Yahoo client — no duplication).
	fxStore := fx.NewPostgresStore(sqlDB)
	fxProvider := data.NewYahooFXProviderWithProvider(yahooProvider)
	fxSvc := fx.NewService(fxProvider, fxStore)

	// Price feed: refresh EOD quotes every 30 minutes and sync dividend income when
	// the store supports the DividendSyncer interface (no-op otherwise).
	// After each refresh a NAV snapshot is recorded so the TWR chart accumulates
	// data points over time. FX service converts multi-currency NAVs to base currency.
	// priceFeed is declared outside the if-block so it can be wired into the root model
	// for hot-reload support (SetBaseCurrency / TriggerBackfill on settings save).
	var priceFeed *feed.Updater

	if priceStore != nil {
		if ds, ok := store.(feed.DividendSyncer); ok {
			priceFeed = feed.NewWithDividendSync(priceStore, ds, candleProvider, priceFeedInterval)
		} else {
			priceFeed = feed.New(priceStore, candleProvider, priceFeedInterval)
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

	rootModel := newRootModelFn(
		store, signalRuntime.ReadModel, runner, txStore, settingsSvc, logCh, twrEngine, baseCurrency,
	)
	// Wire hot-reload support when the concrete RootModel is available.
	// In tests newRootModelFn may return a fake that does not implement these setters,
	// in which case the type assertion returns ok=false and we skip gracefully.
	if rm, ok := rootModel.(signals.RootModel); ok {
		rootModel = rm.WithYahooProvider(yahooProvider).WithUpdater(priceFeed).WithTAAEngine(taaEngine)
	}

	program := newProgramFn(rootModel, tea.WithAltScreen())

	_, runErr := program.Run()
	if runErr != nil {
		return fmt.Errorf("tui runtime failed: %w", runErr)
	}

	return nil
}

func parseConfig(args []string, lookupEnv func(string) string) (config, error) {
	var conf config

	flagSet := flag.NewFlagSet("orbiter", flag.ContinueOnError)
	flagSet.StringVar(&conf.dsn, "dsn", "", "PostgreSQL DSN")

	parseErr := flagSet.Parse(args)
	if parseErr != nil {
		return config{}, fmt.Errorf("parse flags: %w", parseErr)
	}

	if len(flagSet.Args()) > 0 {
		return config{}, fmt.Errorf("%w: %s", ErrUnexpectedArgs, strings.Join(flagSet.Args(), " "))
	}

	conf.dsn = strings.TrimSpace(conf.dsn)
	if conf.dsn == "" {
		conf.dsn = strings.TrimSpace(lookupEnv("DATABASE_URL"))
	}

	if conf.dsn == "" {
		return config{}, ErrMissingDSN
	}

	return conf, nil
}

func openPostgres(ctx context.Context, dsn string) (*sql.DB, error) {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	pingErr := sqlDB.PingContext(ctx)
	if pingErr != nil {
		_ = sqlDB.Close()

		return nil, fmt.Errorf("failed to ping database with DSN %q: %w", dsn, pingErr)
	}

	return sqlDB, nil
}
