//nolint:testpackage // accesses unexported symbols: fakeRepo, newFakeRepo, defaultSettings, setTyped
package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeRepo struct {
	settings map[string]Setting
	countErr error
	setErr   error
	getErr   error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		settings: map[string]Setting{},
		countErr: nil,
		setErr:   nil,
		getErr:   nil,
	}
}

func (f *fakeRepo) Get(_ context.Context, key string) (Setting, error) {
	if f.getErr != nil {
		return Setting{}, f.getErr
	}

	stored, exists := f.settings[key]
	if !exists {
		return Setting{}, ErrSettingNotFound
	}

	return stored, nil
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

	err := SeedDefaultsIfEmpty(ctx, repo)
	if err != nil {
		t.Fatalf("SeedDefaultsIfEmpty() error = %v", err)
	}

	if len(repo.settings) != len(defaultSettings) {
		t.Fatalf("seeded settings count = %d, want %d", len(repo.settings), len(defaultSettings))
	}

	creds, exists := repo.settings[KeyYahooCredentials]
	if !exists {
		t.Fatalf("missing seeded key %s", KeyYahooCredentials)
	}

	var credsValue YahooCredentialsSetting

	err = json.Unmarshal(creds.ValueJSON, &credsValue)
	if err != nil {
		t.Fatalf("unmarshal yahoo credentials setting failed: %v", err)
	}

	if credsValue.APIKey != "" {
		t.Fatalf("default yahoo api key = %q, want empty", credsValue.APIKey)
	}

	currency, exists := repo.settings[KeyPortfolioBaseCurrency]
	if !exists {
		t.Fatalf("missing seeded key %s", KeyPortfolioBaseCurrency)
	}

	var currencyValue PortfolioBaseCurrencySetting

	err = json.Unmarshal(currency.ValueJSON, &currencyValue)
	if err != nil {
		t.Fatalf("unmarshal base currency setting failed: %v", err)
	}

	if currencyValue.Currency != defaultBaseCurrency {
		t.Fatalf("default base currency = %q, want EUR", currencyValue.Currency)
	}
}

func TestSeedDefaultsIfEmptySkipsWhenNotEmpty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()
	repo.settings["existing"] = Setting{
		Key:         "existing",
		Scope:       "",
		Description: "",
		ValueJSON:   []byte(`{"v":1}`),
		CreatedAt:   time.Time{},
		UpdatedAt:   time.Time{},
	}

	err := SeedDefaultsIfEmpty(ctx, repo)
	if err != nil {
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

	err := SeedDefaultsIfEmpty(ctx, repo)
	if err == nil {
		t.Fatalf("SeedDefaultsIfEmpty() error = nil, want non-nil")
	}
}

func TestServiceYahooCredentialsRoundtrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()
	svc := NewService(repo)

	err := svc.SetYahooCredentials(ctx, YahooCredentialsSetting{APIKey: "my-secret-key"})
	if err != nil {
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

	err := svc.SetBaseCurrency(ctx, "USD")
	if err != nil {
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

	err := svc.SetBaseCurrency(ctx, "INVALID")
	if err == nil {
		t.Fatalf("SetBaseCurrency() error = nil for invalid currency, want non-nil")
	}
}

func TestServiceValidateRequired(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()

	err := SeedDefaultsIfEmpty(ctx, repo)
	if err != nil {
		t.Fatalf("SeedDefaultsIfEmpty() error = %v", err)
	}

	svc := NewService(repo)

	err = svc.ValidateRequired(ctx)
	if err != nil {
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

	repo.settings[KeyYahooCredentials] = Setting{
		Key:         KeyYahooCredentials,
		Scope:       "",
		Description: "",
		ValueJSON:   []byte(`not-json`),
		CreatedAt:   time.Time{},
		UpdatedAt:   time.Time{},
	}

	_, err := svc.GetYahooCredentials(ctx)
	if err == nil || !strings.Contains(err.Error(), "cannot decode JSON") {
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
