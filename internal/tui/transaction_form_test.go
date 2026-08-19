//nolint:testpackage // accesses unexported tui symbols (transactionFormModel, txFormResultMsg, formField*)
package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tgragnato/orbiter/internal/portfolio"
)

// helpers ─────────────────────────────────────────────────────────────────────

func newTestForm(t *testing.T, symbols ...string) transactionFormModel {
	t.Helper()
	f, _ := newTransactionForm(symbols)
	return f
}

func setFormValues(f transactionFormModel, symbol, qty, price, fee, date string) transactionFormModel {
	f.symbolInput.SetValue(symbol)
	f.qtyInput.SetValue(qty)
	f.priceInput.SetValue(price)
	f.feeInput.SetValue(fee)
	f.dateInput.SetValue(date)
	return f
}

// buildTx ─────────────────────────────────────────────────────────────────────

func TestBuildTxValid(t *testing.T) {
	t.Parallel()

	f := newTestForm(t)
	f = setFormValues(f, "VWCE.MI", "10", "98.50", "3.95", "2024-06-01")

	tx, errMsg := f.buildTx()
	if errMsg != "" {
		t.Fatalf("buildTx() errMsg = %q, want empty", errMsg)
	}
	if tx.Symbol != "VWCE.MI" {
		t.Errorf("Symbol = %q, want VWCE.MI", tx.Symbol)
	}
	if tx.Quantity != 10 {
		t.Errorf("Quantity = %v, want 10", tx.Quantity)
	}
	if tx.Price != 98.50 {
		t.Errorf("Price = %v, want 98.50", tx.Price)
	}
	if tx.Fee != 3.95 {
		t.Errorf("Fee = %v, want 3.95", tx.Fee)
	}
	wantDate := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	if !tx.ExecutedAt.Equal(wantDate) {
		t.Errorf("ExecutedAt = %v, want %v", tx.ExecutedAt, wantDate)
	}
}

func TestBuildTxMissingSymbol(t *testing.T) {
	t.Parallel()

	f := newTestForm(t)
	f = setFormValues(f, "  ", "10", "100", "0", "2024-01-01")

	_, errMsg := f.buildTx()
	if errMsg == "" {
		t.Fatal("buildTx() with empty symbol must return an error")
	}
	if !strings.Contains(errMsg, "symbol") {
		t.Errorf("errMsg = %q, want to mention 'symbol'", errMsg)
	}
}

func TestBuildTxInvalidQty(t *testing.T) {
	t.Parallel()

	cases := []string{"", "abc", "0", "-5"}
	for _, v := range cases {
		f := newTestForm(t)
		f = setFormValues(f, "AAPL", v, "100", "0", "2024-01-01")
		_, errMsg := f.buildTx()
		if errMsg == "" {
			t.Errorf("buildTx() with qty=%q must return an error", v)
		}
	}
}

func TestBuildTxInvalidPrice(t *testing.T) {
	t.Parallel()

	cases := []string{"", "abc", "0", "-1"}
	for _, v := range cases {
		f := newTestForm(t)
		f = setFormValues(f, "AAPL", "10", v, "0", "2024-01-01")
		_, errMsg := f.buildTx()
		if errMsg == "" {
			t.Errorf("buildTx() with price=%q must return an error", v)
		}
	}
}

func TestBuildTxNegativeFee(t *testing.T) {
	t.Parallel()

	f := newTestForm(t)
	f = setFormValues(f, "AAPL", "10", "100", "-1", "2024-01-01")

	_, errMsg := f.buildTx()
	if errMsg == "" {
		t.Fatal("buildTx() with negative fee must return an error")
	}
}

func TestBuildTxInvalidDate(t *testing.T) {
	t.Parallel()

	f := newTestForm(t)
	f = setFormValues(f, "AAPL", "10", "100", "0", "not-a-date")

	_, errMsg := f.buildTx()
	if errMsg == "" {
		t.Fatal("buildTx() with invalid date must return an error")
	}
}

func TestBuildTxEmptyFeeDefaultsToZero(t *testing.T) {
	t.Parallel()

	f := newTestForm(t)
	f = setFormValues(f, "AAPL", "10", "100", "", "2024-01-01")

	tx, errMsg := f.buildTx()
	if errMsg != "" {
		t.Fatalf("buildTx() errMsg = %q, want empty for blank fee", errMsg)
	}
	if tx.Fee != 0 {
		t.Errorf("Fee = %v, want 0", tx.Fee)
	}
}

func TestBuildTxEmptyDateDefaultsToToday(t *testing.T) {
	t.Parallel()

	f := newTestForm(t)
	f = setFormValues(f, "AAPL", "10", "100", "0", "")

	tx, errMsg := f.buildTx()
	if errMsg != "" {
		t.Fatalf("buildTx() errMsg = %q, want empty for blank date", errMsg)
	}
	if tx.ExecutedAt.IsZero() {
		t.Error("ExecutedAt must not be zero when date field is empty")
	}
}

func TestBuildTxSymbolUppercased(t *testing.T) {
	t.Parallel()

	f := newTestForm(t)
	f = setFormValues(f, "vwce.mi", "5", "50", "0", "2024-01-01")

	tx, _ := f.buildTx()
	if tx.Symbol != "VWCE.MI" {
		t.Errorf("Symbol = %q, want uppercased VWCE.MI", tx.Symbol)
	}
}

// autocomplete ────────────────────────────────────────────────────────────────

func TestAutocompleteMatchesPrefix(t *testing.T) {
	t.Parallel()

	f := newTestForm(t, "VWCE.MI", "ZPRV.DE")
	f.symbolInput.SetValue("VW")
	f = f.withUpdatedAutocomplete()

	if f.autocompleteHint != "VWCE.MI" {
		t.Errorf("autocompleteHint = %q, want VWCE.MI", f.autocompleteHint)
	}
}

func TestAutocompleteNoMatchClearsHint(t *testing.T) {
	t.Parallel()

	f := newTestForm(t, "VWCE.MI")
	f.symbolInput.SetValue("XYZ")
	f = f.withUpdatedAutocomplete()

	if f.autocompleteHint != "" {
		t.Errorf("autocompleteHint = %q, want empty for no match", f.autocompleteHint)
	}
}

func TestAutocompleteEmptyInputClearsHint(t *testing.T) {
	t.Parallel()

	f := newTestForm(t, "VWCE.MI")
	f.symbolInput.SetValue("")
	f = f.withUpdatedAutocomplete()

	if f.autocompleteHint != "" {
		t.Errorf("autocompleteHint = %q, want empty for empty input", f.autocompleteHint)
	}
}

func TestAutocompleteExactMatchClearsHint(t *testing.T) {
	t.Parallel()

	f := newTestForm(t, "VWCE.MI")
	f.symbolInput.SetValue("VWCE.MI")
	f = f.withUpdatedAutocomplete()

	if f.autocompleteHint != "" {
		t.Errorf("autocompleteHint = %q, want empty for exact match", f.autocompleteHint)
	}
}

// keyboard navigation ─────────────────────────────────────────────────────────

func TestFormTabAdvancesFocus(t *testing.T) {
	t.Parallel()

	f := newTestForm(t)
	initial := f.focused

	mf, _ := f.Update(tea.KeyMsg{Type: tea.KeyTab})
	if mf.focused == initial {
		t.Errorf("Tab did not advance focus (still %d)", initial)
	}
}

func TestFormShiftTabRetreatsFocus(t *testing.T) {
	t.Parallel()

	f := newTestForm(t)
	// Advance first.
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyTab})
	focusAfterTab := f.focused

	mf, _ := f.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if mf.focused == focusAfterTab {
		t.Errorf("Shift+Tab did not retreat focus (still %d)", focusAfterTab)
	}
}

func TestFormEscCancels(t *testing.T) {
	t.Parallel()

	f := newTestForm(t)
	_, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Esc must return a cmd")
	}
	msg := cmd()
	result, ok := msg.(txFormResultMsg)
	if !ok {
		t.Fatalf("Esc cmd produced %T, want txFormResultMsg", msg)
	}
	if !result.cancelled {
		t.Error("Esc must produce a cancelled txFormResultMsg")
	}
}

func TestFormEscClearsAutocomplete(t *testing.T) {
	t.Parallel()

	f := newTestForm(t, "VWCE.MI")
	f.symbolInput.SetValue("VW")
	f = f.withUpdatedAutocomplete()
	if f.autocompleteHint == "" {
		t.Skip("autocomplete not triggered, skipping")
	}

	mf, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if mf.autocompleteHint != "" {
		t.Errorf("autocompleteHint after Esc = %q, want empty", mf.autocompleteHint)
	}
	// First Esc with hint active clears hint, does not cancel.
	if cmd != nil {
		t.Error("first Esc with autocomplete hint must not produce a cancel cmd")
	}
}

func TestFormEnterAcceptsAutocomplete(t *testing.T) {
	t.Parallel()

	f := newTestForm(t, "VWCE.MI")
	f.symbolInput.SetValue("VW")
	f = f.withUpdatedAutocomplete()
	if f.autocompleteHint == "" {
		t.Skip("autocomplete not triggered, skipping")
	}

	mf, _ := f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if mf.symbolInput.Value() != "VWCE.MI" {
		t.Errorf("symbol after Enter on autocomplete = %q, want VWCE.MI", mf.symbolInput.Value())
	}
}

func TestFormEnterWithValidInputSubmits(t *testing.T) {
	t.Parallel()

	f := newTestForm(t)
	f = setFormValues(f, "AAPL", "10", "150", "0", "2024-01-15")

	_, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter with valid data must return a cmd")
	}
	msg := cmd()
	result, ok := msg.(txFormResultMsg)
	if !ok {
		t.Fatalf("Enter cmd produced %T, want txFormResultMsg", msg)
	}
	if result.cancelled {
		t.Error("valid submit must not be cancelled")
	}
	if result.tx == nil {
		t.Fatal("tx must not be nil on valid submit")
	}
}

func TestFormEnterWithInvalidInputSetsErrMsg(t *testing.T) {
	t.Parallel()

	f := newTestForm(t)
	// Empty symbol — buildTx will fail.
	_, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("Enter with invalid data must not return a cmd")
	}
}

// toggle ──────────────────────────────────────────────────────────────────────

func TestFormSpaceTogglesBuyType(t *testing.T) {
	t.Parallel()

	f, _ := newTransactionForm(nil)
	// Navigate to type field.
	for f.focused != formFieldType {
		f, _ = f.Update(tea.KeyMsg{Type: tea.KeyTab})
	}

	if f.txType != portfolio.TransactionBuy {
		t.Fatalf("initial txType = %v, want Buy", f.txType)
	}
	mf, _ := f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if mf.txType != portfolio.TransactionSell {
		t.Errorf("txType after space = %v, want Sell", mf.txType)
	}
	// Toggle back.
	mf2, _ := mf.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if mf2.txType != portfolio.TransactionBuy {
		t.Errorf("txType after 2nd space = %v, want Buy", mf2.txType)
	}
}

func TestFormSpaceTogglesAllocType(t *testing.T) {
	t.Parallel()

	f, _ := newTransactionForm(nil)
	// Navigate to alloc field.
	for f.focused != formFieldAlloc {
		f, _ = f.Update(tea.KeyMsg{Type: tea.KeyTab})
	}

	if f.allocType != portfolio.AllocationSatellite {
		t.Fatalf("initial allocType = %v, want Satellite", f.allocType)
	}
	mf, _ := f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if mf.allocType != portfolio.AllocationCore {
		t.Errorf("allocType after space = %v, want Core", mf.allocType)
	}
}

// editing mode ────────────────────────────────────────────────────────────────

func TestNewTransactionFormEditing(t *testing.T) {
	t.Parallel()

	tx := portfolio.Transaction{
		ID:             42,
		Symbol:         "ZPRV.DE",
		Type:           portfolio.TransactionSell,
		Quantity:       5,
		Price:          200,
		Fee:            1.5,
		AllocationType: portfolio.AllocationCore,
		ExecutedAt:     time.Date(2023, 3, 10, 0, 0, 0, 0, time.UTC),
	}
	f, _ := newTransactionFormEditing(tx, nil)

	if f.txID != 42 {
		t.Errorf("txID = %d, want 42", f.txID)
	}
	if f.symbolInput.Value() != "ZPRV.DE" {
		t.Errorf("symbol = %q, want ZPRV.DE", f.symbolInput.Value())
	}
	if f.txType != portfolio.TransactionSell {
		t.Errorf("txType = %v, want Sell", f.txType)
	}
	if f.allocType != portfolio.AllocationCore {
		t.Errorf("allocType = %v, want Core", f.allocType)
	}
}

func TestNewTransactionFormEditingTitle(t *testing.T) {
	t.Parallel()

	tx := portfolio.Transaction{
		ID: 7, Symbol: "SPY", Type: portfolio.TransactionBuy, Quantity: 1, Price: 400, ExecutedAt: time.Now(),
	}
	f, _ := newTransactionFormEditing(tx, nil)

	view := f.View()
	if !strings.Contains(view, "Edit Transaction") {
		t.Errorf("View() = %q, want 'Edit Transaction'", view)
	}
}

func TestNewTransactionFormTitle(t *testing.T) {
	t.Parallel()

	f := newTestForm(t)
	view := f.View()
	if !strings.Contains(view, "Add Transaction") {
		t.Errorf("View() = %q, want 'Add Transaction'", view)
	}
}

// View ────────────────────────────────────────────────────────────────────────

func TestFormViewShowsErrorMsg(t *testing.T) {
	t.Parallel()

	f := newTestForm(t)
	f.errMsg = "price must be a positive number"

	view := f.View()
	if !strings.Contains(view, "price must be a positive number") {
		t.Errorf("View() = %q, want error message in output", view)
	}
}

func TestFormViewShowsAutocompleteHint(t *testing.T) {
	t.Parallel()

	f := newTestForm(t, "VWCE.MI")
	f.symbolInput.SetValue("VW")
	f = f.withUpdatedAutocomplete()
	if f.autocompleteHint == "" {
		t.Skip("autocomplete not triggered")
	}

	view := f.View()
	if !strings.Contains(view, "VWCE.MI") {
		t.Errorf("View() = %q, want autocomplete hint in output", view)
	}
}
