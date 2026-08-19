//nolint:testpackage // accesses unexported package-level vars for test injection
package startup

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tgragnato/orbiter/internal/configuration"
	"github.com/tgragnato/orbiter/internal/portfolio"
	"github.com/tgragnato/orbiter/internal/portfolio/analytics"
	"github.com/tgragnato/orbiter/internal/signal"
	"github.com/tgragnato/orbiter/internal/tui"
)

const (
	testDSNFlag      = "--dsn"
	testDSNFlagValue = "postgres://flag"
	testDSNEnvValue  = "postgres://env"
)

type fakeProgram struct {
	runErr   error
	runCalls int
}

func (p *fakeProgram) Run() (tea.Model, error) {
	p.runCalls++

	return nil, p.runErr
}

//nolint:funlen // table-driven test with many cases
func TestParseConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		env        map[string]string
		wantDSN    string
		wantErr    bool
		errContain string
	}{
		{
			name:       "dsn from flag",
			args:       []string{testDSNFlag, testDSNFlagValue},
			wantDSN:    testDSNFlagValue,
			env:        nil,
			wantErr:    false,
			errContain: "",
		},
		{
			name:       "dsn from environment",
			env:        map[string]string{"DATABASE_URL": testDSNEnvValue},
			wantDSN:    testDSNEnvValue,
			args:       nil,
			wantErr:    false,
			errContain: "",
		},
		{
			name:       "flag takes precedence",
			args:       []string{testDSNFlag, testDSNFlagValue},
			env:        map[string]string{"DATABASE_URL": testDSNEnvValue},
			wantDSN:    testDSNFlagValue,
			wantErr:    false,
			errContain: "",
		},
		{
			name:       "missing dsn",
			wantErr:    true,
			errContain: "missing PostgreSQL DSN",
			args:       nil,
			env:        nil,
			wantDSN:    "",
		},
		{
			name:       "unexpected positional argument",
			args:       []string{"unexpected"},
			wantErr:    true,
			errContain: "unexpected positional arguments",
			env:        nil,
			wantDSN:    "",
		},
		{
			name:       "unknown flag",
			args:       []string{"--instrument", "EURUSD"},
			wantErr:    true,
			errContain: "flag provided but not defined",
			env:        nil,
			wantDSN:    "",
		},
		{
			name:       "legacy db dsn flag rejected",
			args:       []string{"--db-dsn", "postgres://legacy"},
			wantErr:    true,
			errContain: "flag provided but not defined",
			env:        nil,
			wantDSN:    "",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			lookup := func(key string) string {
				if testCase.env == nil {
					return ""
				}

				return testCase.env[key]
			}

			conf, err := parseConfig(testCase.args, lookup)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("parseConfig() error = nil, want non-nil")
				}

				if testCase.errContain != "" && !strings.Contains(err.Error(), testCase.errContain) {
					t.Fatalf("parseConfig() error = %q, want substring %q", err.Error(), testCase.errContain)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseConfig() error = %v", err)
			}

			if conf.dsn != testCase.wantDSN {
				t.Fatalf("dsn = %q, want %q", conf.dsn, testCase.wantDSN)
			}
		})
	}
}

//nolint:paralleltest,cyclop,funlen // uses package-level test vars; t.Cleanup restores them
func TestRunUsesOpenAndBootstrap(t *testing.T) {
	origOpen := openPostgresFn
	origBootstrap := bootstrapFn
	origSignalRuntime := newSignalRTFn
	origStore := newStoreFn
	origRootModel := newRootModelFn
	origProgram := newProgramFn

	t.Cleanup(func() {
		openPostgresFn = origOpen
		bootstrapFn = origBootstrap
		newSignalRTFn = origSignalRuntime
		newStoreFn = origStore
		newRootModelFn = origRootModel
		newProgramFn = origProgram
	})

	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}

	defer sqlDB.Close()

	openCalls := 0
	bootCalls := 0
	storeCalls := 0
	rootCalls := 0
	programCalls := 0
	fakeProg := &fakeProgram{runErr: nil, runCalls: 0}

	openPostgresFn = func(_ context.Context, dsn string) (*sql.DB, error) {
		openCalls++

		if dsn != testDSNFlagValue {
			t.Fatalf("dsn = %q, want %s", dsn, testDSNFlagValue)
		}

		return sqlDB, nil
	}
	bootstrapFn = func(_ context.Context, gotDB *sql.DB) (*configuration.Service, error) {
		bootCalls++

		if gotDB != sqlDB {
			t.Fatalf("bootstrap db mismatch")
		}

		return nil, nil //nolint:nilnil // test stub: bootstrap returns nil service and nil error
	}
	newSignalRTFn = signal.NewRuntime
	newStoreFn = func(gotDB *sql.DB) portfolio.HoldingsStore {
		storeCalls++

		if gotDB != sqlDB {
			t.Fatalf("store db mismatch")
		}

		return &fakeHoldingsStoreForStartup{}
	}
	newRootModelFn = func(
		store portfolio.HoldingsStore,
		readModel signal.ReadModel,
		_ tui.MLEngine,
		_ portfolio.TransactionStore,
		_ tui.SettingsService,
		_ tui.LogChannel,
		_ *analytics.TWREngine,
		_ string,
	) tea.Model {
		rootCalls++

		if store == nil {
			t.Fatalf("store = nil")
		}

		if readModel == nil {
			t.Fatalf("readModel = nil")
		}

		return stubTeaModel{}
	}
	newProgramFn = func(model tea.Model, options ...tea.ProgramOption) programRunner {
		programCalls++

		if model == nil {
			t.Fatalf("program model = nil")
		}

		if len(options) != 1 {
			t.Fatalf("program options len = %d, want 1", len(options))
		}

		return fakeProg
	}

	runErr := Run(context.Background(), []string{testDSNFlag, testDSNFlagValue})
	if runErr != nil {
		t.Fatalf("Run() error = %v", runErr)
	}

	if openCalls != 1 {
		t.Fatalf("open calls = %d, want 1", openCalls)
	}

	if bootCalls != 1 {
		t.Fatalf("bootstrap calls = %d, want 1", bootCalls)
	}

	if storeCalls != 1 {
		t.Fatalf("store calls = %d, want 1", storeCalls)
	}

	if rootCalls != 1 {
		t.Fatalf("root model calls = %d, want 1", rootCalls)
	}

	if programCalls != 1 {
		t.Fatalf("program calls = %d, want 1", programCalls)
	}

	if fakeProg.runCalls != 1 {
		t.Fatalf("program run calls = %d, want 1", fakeProg.runCalls)
	}
}

//nolint:paralleltest // uses package-level test vars; t.Cleanup restores them
func TestRunOpenError(t *testing.T) {
	origOpen := openPostgresFn
	origBootstrap := bootstrapFn
	origSignalRuntime := newSignalRTFn
	origStore := newStoreFn
	origRootModel := newRootModelFn
	origProgram := newProgramFn

	t.Cleanup(func() {
		openPostgresFn = origOpen
		bootstrapFn = origBootstrap
		newSignalRTFn = origSignalRuntime
		newStoreFn = origStore
		newRootModelFn = origRootModel
		newProgramFn = origProgram
	})

	openPostgresFn = func(_ context.Context, _ string) (*sql.DB, error) {
		return nil, errors.New("open failed")
	}
	bootstrapFn = func(_ context.Context, _ *sql.DB) (*configuration.Service, error) {
		t.Fatalf("bootstrap should not be called")

		return nil, nil //nolint:nilnil // unreachable; satisfies compiler
	}
	newProgramFn = func(model tea.Model, options ...tea.ProgramOption) programRunner {
		t.Fatalf("program should not be created")

		return &fakeProgram{runErr: nil, runCalls: 0}
	}

	err := Run(context.Background(), []string{testDSNFlag, testDSNFlagValue})
	if err == nil || !strings.Contains(err.Error(), "open failed") {
		t.Fatalf("Run() error = %v, want open failed", err)
	}
}

//nolint:paralleltest // uses package-level test vars; t.Cleanup restores them
func TestRunBootstrapError(t *testing.T) {
	origOpen := openPostgresFn
	origBootstrap := bootstrapFn
	origSignalRuntime := newSignalRTFn
	origStore := newStoreFn
	origRootModel := newRootModelFn
	origProgram := newProgramFn

	t.Cleanup(func() {
		openPostgresFn = origOpen
		bootstrapFn = origBootstrap
		newSignalRTFn = origSignalRuntime
		newStoreFn = origStore
		newRootModelFn = origRootModel
		newProgramFn = origProgram
	})

	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}

	defer sqlDB.Close()

	openPostgresFn = func(_ context.Context, _ string) (*sql.DB, error) {
		return sqlDB, nil
	}
	bootstrapFn = func(_ context.Context, _ *sql.DB) (*configuration.Service, error) {
		return nil, errors.New("bootstrap failed")
	}
	newProgramFn = func(model tea.Model, options ...tea.ProgramOption) programRunner {
		t.Fatalf("program should not be created")

		return &fakeProgram{runErr: nil, runCalls: 0}
	}

	err = Run(context.Background(), []string{testDSNFlag, testDSNFlagValue})
	if err == nil || !strings.Contains(err.Error(), "configuration bootstrap failed") {
		t.Fatalf("Run() error = %v, want configuration bootstrap failed", err)
	}
}

//nolint:paralleltest,funlen // uses package-level test vars; t.Cleanup restores them; setup boilerplate pushes length
func TestRunProgramError(t *testing.T) {
	origOpen := openPostgresFn
	origBootstrap := bootstrapFn
	origSignalRuntime := newSignalRTFn
	origStore := newStoreFn
	origRootModel := newRootModelFn
	origProgram := newProgramFn

	t.Cleanup(func() {
		openPostgresFn = origOpen
		bootstrapFn = origBootstrap
		newSignalRTFn = origSignalRuntime
		newStoreFn = origStore
		newRootModelFn = origRootModel
		newProgramFn = origProgram
	})

	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}

	defer sqlDB.Close()

	openPostgresFn = func(_ context.Context, _ string) (*sql.DB, error) {
		return sqlDB, nil
	}
	bootstrapFn = func(_ context.Context, _ *sql.DB) (*configuration.Service, error) {
		return nil, nil //nolint:nilnil // test stub: bootstrap returns nil service and nil error
	}
	newSignalRTFn = signal.NewRuntime
	newStoreFn = func(_ *sql.DB) portfolio.HoldingsStore { return &fakeHoldingsStoreForStartup{} }
	newRootModelFn = func(
		store portfolio.HoldingsStore,
		readModel signal.ReadModel,
		mlEngine tui.MLEngine,
		txStore portfolio.TransactionStore,
		configSvc tui.SettingsService,
		logCh tui.LogChannel,
		twrEngine *analytics.TWREngine,
		_ string,
	) tea.Model {
		_ = store
		_ = readModel
		_ = mlEngine
		_ = txStore
		_ = configSvc
		_ = logCh
		_ = twrEngine

		return stubTeaModel{}
	}
	newProgramFn = func(model tea.Model, options ...tea.ProgramOption) programRunner {
		_ = model
		_ = options

		return &fakeProgram{runErr: errors.New("ui failed"), runCalls: 0}
	}

	err = Run(context.Background(), []string{testDSNFlag, testDSNFlagValue})
	if err == nil || !strings.Contains(err.Error(), "tui runtime failed") {
		t.Fatalf("Run() error = %v, want tui runtime failed", err)
	}
}

type fakeHoldingsStoreForStartup struct{}

func (f *fakeHoldingsStoreForStartup) ListHoldings(context.Context) ([]portfolio.Holding, error) {
	return nil, nil
}

func (f *fakeHoldingsStoreForStartup) ToggleAllocation(context.Context, int64) error {
	return nil
}

func (f *fakeHoldingsStoreForStartup) ToggleTAAEnabled(context.Context, string) error {
	return nil
}

func (f *fakeHoldingsStoreForStartup) TotalRealizedPnL(context.Context) (float64, error) {
	return 0, nil
}

type stubTeaModel struct{}

func (stubTeaModel) Init() tea.Cmd { return nil }

func (stubTeaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_ = msg

	return stubTeaModel{}, nil
}

func (stubTeaModel) View() string { return "" }

func TestOpenPostgresError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := openPostgres(ctx, "postgres://invalid:invalid@127.0.0.1:1/at?sslmode=disable")
	if err == nil {
		t.Fatalf("openPostgres() error = nil, want non-nil")
	}
}
