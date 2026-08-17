package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakeRepo struct {
	settings map[string]Setting
	countErr error
	setErr   error
	getErr   error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{settings: map[string]Setting{}}
}

func (f *fakeRepo) Get(_ context.Context, key string) (Setting, error) {
	if f.getErr != nil {
		return Setting{}, f.getErr
	}
	setting, ok := f.settings[key]
	if !ok {
		return Setting{}, ErrSettingNotFound
	}
	return setting, nil
}

func (f *fakeRepo) Set(_ context.Context, setting Setting) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.settings[setting.Key] = setting
	return nil
}

func (f *fakeRepo) List(_ context.Context) ([]Setting, error) {
	settings := make([]Setting, 0, len(f.settings))
	for _, s := range f.settings {
		settings = append(settings, s)
	}
	return settings, nil
}

func (f *fakeRepo) Count(_ context.Context) (int, error) {
	if f.countErr != nil {
		return 0, f.countErr
	}
	return len(f.settings), nil
}

func TestSeedDefaultsIfEmpty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()

	if err := SeedDefaultsIfEmpty(ctx, repo); err != nil {
		t.Fatalf("SeedDefaultsIfEmpty() error = %v", err)
	}

	if len(repo.settings) != len(defaultSettings) {
		t.Fatalf("seeded settings count = %d, want %d", len(repo.settings), len(defaultSettings))
	}

	creds, ok := repo.settings[KeyYahooCredentials]
	if !ok {
		t.Fatalf("missing seeded key %s", KeyYahooCredentials)
	}

	var credsValue YahooCredentialsSetting
	if err := json.Unmarshal(creds.ValueJSON, &credsValue); err != nil {
		t.Fatalf("unmarshal yahoo credentials setting failed: %v", err)
	}
	if credsValue.APIKey != "" {
		t.Fatalf("default yahoo api key = %q, want empty", credsValue.APIKey)
	}

	currency, ok := repo.settings[KeyPortfolioBaseCurrency]
	if !ok {
		t.Fatalf("missing seeded key %s", KeyPortfolioBaseCurrency)
	}

	var currencyValue PortfolioBaseCurrencySetting
	if err := json.Unmarshal(currency.ValueJSON, &currencyValue); err != nil {
		t.Fatalf("unmarshal base currency setting failed: %v", err)
	}
	if currencyValue.Currency != "EUR" {
		t.Fatalf("default base currency = %q, want EUR", currencyValue.Currency)
	}
}

func TestSeedDefaultsIfEmptySkipsWhenNotEmpty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()
	repo.settings["existing"] = Setting{Key: "existing", ValueJSON: []byte(`{"v":1}`)}

	if err := SeedDefaultsIfEmpty(ctx, repo); err != nil {
		t.Fatalf("SeedDefaultsIfEmpty() error = %v", err)
	}
	if len(repo.settings) != 1 {
		t.Fatalf("settings size = %d, want 1", len(repo.settings))
	}
}

func TestSeedDefaultsIfEmptyCountError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()
	repo.countErr = errors.New("boom")

	if err := SeedDefaultsIfEmpty(ctx, repo); err == nil {
		t.Fatalf("SeedDefaultsIfEmpty() error = nil, want non-nil")
	}
}

func TestServiceYahooCredentialsRoundtrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()
	svc := NewService(repo)

	if err := svc.SetYahooCredentials(ctx, YahooCredentialsSetting{APIKey: "my-secret-key"}); err != nil {
		t.Fatalf("SetYahooCredentials() error = %v", err)
	}

	creds, err := svc.GetYahooCredentials(ctx)
	if err != nil {
		t.Fatalf("GetYahooCredentials() error = %v", err)
	}
	if creds.APIKey != "my-secret-key" {
		t.Fatalf("APIKey = %q, want my-secret-key", creds.APIKey)
	}
}

func TestServiceBaseCurrencyRoundtrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()
	svc := NewService(repo)

	if err := svc.SetBaseCurrency(ctx, "USD"); err != nil {
		t.Fatalf("SetBaseCurrency() error = %v", err)
	}

	curr, err := svc.GetBaseCurrency(ctx)
	if err != nil {
		t.Fatalf("GetBaseCurrency() error = %v", err)
	}
	if curr != "USD" {
		t.Fatalf("currency = %q, want USD", curr)
	}
}

func TestServiceBaseCurrencyValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()
	svc := NewService(repo)

	if err := svc.SetBaseCurrency(ctx, "INVALID"); err == nil {
		t.Fatalf("SetBaseCurrency() error = nil for invalid currency, want non-nil")
	}
}

func TestServiceValidateRequired(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()
	if err := SeedDefaultsIfEmpty(ctx, repo); err != nil {
		t.Fatalf("SeedDefaultsIfEmpty() error = %v", err)
	}

	svc := NewService(repo)
	if err := svc.ValidateRequired(ctx); err != nil {
		t.Fatalf("ValidateRequired() error = %v", err)
	}
}

func TestServiceValidateRequiredMissing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()
	svc := NewService(repo)

	err := svc.ValidateRequired(ctx)
	if err == nil {
		t.Fatalf("ValidateRequired() error = nil, want non-nil")
	}
}

func TestServiceInvalidStoredSettings(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()
	svc := NewService(repo)

	repo.settings[KeyYahooCredentials] = Setting{Key: KeyYahooCredentials, ValueJSON: []byte(`not-json`)}
	if _, err := svc.GetYahooCredentials(ctx); err == nil || !strings.Contains(err.Error(), "cannot decode JSON") {
		t.Fatalf("GetYahooCredentials() error = %v, want decode error", err)
	}
}

func TestSetTypedMarshalError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()

	// Channels cannot be JSON-marshaled and trigger the marshal error branch.
	err := setTyped(ctx, repo, "invalid", "global", "desc", map[string]any{"bad": make(chan int)})
	if err == nil {
		t.Fatalf("setTyped() error = nil, want non-nil")
	}
}
