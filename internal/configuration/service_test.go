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

	cost, ok := repo.settings[KeyCostBasisMethod]
	if !ok {
		t.Fatalf("missing seeded key %s", KeyCostBasisMethod)
	}

	var costValue CostBasisSetting
	if err := json.Unmarshal(cost.ValueJSON, &costValue); err != nil {
		t.Fatalf("unmarshal cost basis setting failed: %v", err)
	}
	if costValue.Method != CostBasisPMC {
		t.Fatalf("default cost basis = %q, want %q", costValue.Method, CostBasisPMC)
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

func TestServiceCostBasisRoundtrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()
	svc := NewService(repo)

	if err := svc.SetCostBasisMethod(ctx, CostBasisFIFO); err != nil {
		t.Fatalf("SetCostBasisMethod() error = %v", err)
	}

	method, err := svc.GetCostBasisMethod(ctx)
	if err != nil {
		t.Fatalf("GetCostBasisMethod() error = %v", err)
	}
	if method != CostBasisFIFO {
		t.Fatalf("method = %q, want %q", method, CostBasisFIFO)
	}
}

func TestServiceRejectsInvalidCostBasis(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()
	svc := NewService(repo)

	if err := svc.SetCostBasisMethod(ctx, CostBasisMethod("INVALID")); err == nil {
		t.Fatalf("SetCostBasisMethod() error = nil, want non-nil")
	}
}

func TestServiceTargetsValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()
	svc := NewService(repo)

	err := svc.SetCoreSatelliteTargets(ctx, CoreSatelliteTargetSetting{CoreRatio: 0.7, SatelliteRatio: 0.2})
	if err == nil {
		t.Fatalf("SetCoreSatelliteTargets() error = nil, want non-nil")
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

func TestServiceSettersAndGetters(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()
	svc := NewService(repo)

	if err := svc.SetDataProvider(ctx, DataProviderSetting{Provider: "YAHOO", Currency: "EUR"}); err != nil {
		t.Fatalf("SetDataProvider() error = %v", err)
	}
	provider, err := svc.GetDataProvider(ctx)
	if err != nil {
		t.Fatalf("GetDataProvider() error = %v", err)
	}
	if provider.Provider != "YAHOO" || provider.Currency != "EUR" {
		t.Fatalf("provider = %#v, want YAHOO/EUR", provider)
	}

	if err := svc.SetTAA(ctx, TAASetting{RebalanceThreshold: 0.1}); err != nil {
		t.Fatalf("SetTAA() error = %v", err)
	}
	taa, err := svc.GetTAA(ctx)
	if err != nil {
		t.Fatalf("GetTAA() error = %v", err)
	}
	if taa.RebalanceThreshold != 0.1 {
		t.Fatalf("threshold = %v, want 0.1", taa.RebalanceThreshold)
	}

	if err := svc.SetCoreSatelliteTargets(ctx, CoreSatelliteTargetSetting{CoreRatio: 0.6, SatelliteRatio: 0.4}); err != nil {
		t.Fatalf("SetCoreSatelliteTargets() error = %v", err)
	}
	targets, err := svc.GetCoreSatelliteTargets(ctx)
	if err != nil {
		t.Fatalf("GetCoreSatelliteTargets() error = %v", err)
	}
	if targets.CoreRatio != 0.6 || targets.SatelliteRatio != 0.4 {
		t.Fatalf("targets = %#v, want 0.6/0.4", targets)
	}

	if err := svc.SetTUIPreferences(ctx, TUIPreferenceSetting{ShowPercentages: false, NumberFormat: "4dp"}); err != nil {
		t.Fatalf("SetTUIPreferences() error = %v", err)
	}
	tui, err := svc.GetTUIPreferences(ctx)
	if err != nil {
		t.Fatalf("GetTUIPreferences() error = %v", err)
	}
	if tui.ShowPercentages || tui.NumberFormat != "4dp" {
		t.Fatalf("tui = %#v, want false/4dp", tui)
	}

	if err := svc.SetYahooCredentials(ctx, YahooCredentialsSetting{APIKey: "token"}); err != nil {
		t.Fatalf("SetYahooCredentials() error = %v", err)
	}
	creds, err := svc.GetYahooCredentials(ctx)
	if err != nil {
		t.Fatalf("GetYahooCredentials() error = %v", err)
	}
	if creds.APIKey != "token" {
		t.Fatalf("api key = %q, want token", creds.APIKey)
	}
}

func TestServiceValidationErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()
	svc := NewService(repo)

	if err := svc.SetDataProvider(ctx, DataProviderSetting{Provider: "", Currency: "EUR"}); err == nil {
		t.Fatalf("SetDataProvider() error = nil, want non-nil")
	}
	if err := svc.SetTAA(ctx, TAASetting{RebalanceThreshold: 1.0}); err == nil {
		t.Fatalf("SetTAA() error = nil, want non-nil")
	}
	if err := svc.SetTUIPreferences(ctx, TUIPreferenceSetting{NumberFormat: ""}); err == nil {
		t.Fatalf("SetTUIPreferences() error = nil, want non-nil")
	}
}

func TestServiceInvalidStoredSettings(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := newFakeRepo()
	svc := NewService(repo)

	repo.settings[KeyCostBasisMethod] = Setting{Key: KeyCostBasisMethod, ValueJSON: []byte(`{"method":"INVALID"}`)}
	if _, err := svc.GetCostBasisMethod(ctx); err == nil {
		t.Fatalf("GetCostBasisMethod() error = nil, want non-nil")
	}

	repo.settings[KeyDataProvider] = Setting{Key: KeyDataProvider, ValueJSON: []byte(`not-json`)}
	if _, err := svc.GetDataProvider(ctx); err == nil || !strings.Contains(err.Error(), "cannot decode JSON") {
		t.Fatalf("GetDataProvider() error = %v, want decode error", err)
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
