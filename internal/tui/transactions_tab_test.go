//nolint:testpackage // accesses unexported tui symbols (txsLoadedMsg, txAddedMsg, txDisplayEntry, fakeEditor)
package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tgragnato/orbiter/internal/portfolio"
)

// fakeTransactionEditor ───────────────────────────────────────────────────────

type fakeTransactionEditor struct {
	txs     []portfolio.Transaction
	divs    []portfolio.DividendRecord
	listErr error
	addErr  error
	updErr  error
	delErr  error
}

func (f *fakeTransactionEditor) ListTransactions(_ context.Context, _ string) ([]portfolio.Transaction, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]portfolio.Transaction, len(f.txs))
	copy(out, f.txs)
	return out, nil
}

func (f *fakeTransactionEditor) ListDividendIncome(_ context.Context) ([]portfolio.DividendRecord, error) {
	return f.divs, nil
}

func (f *fakeTransactionEditor) AddTransaction(_ context.Context, tx portfolio.Transaction) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.txs = append(f.txs, tx)
	return nil
}

func (f *fakeTransactionEditor) UpdateTransaction(_ context.Context, tx portfolio.Transaction) error {
	if f.updErr != nil {
		return f.updErr
	}
	for i := range f.txs {
		if f.txs[i].ID == tx.ID {
			f.txs[i] = tx
		}
	}
	return nil
}

func (f *fakeTransactionEditor) DeleteTransaction(_ context.Context, id int64) error {
	if f.delErr != nil {
		return f.delErr
	}
	for i := range f.txs {
		if f.txs[i].ID == id {
			f.txs = append(f.txs[:i], f.txs[i+1:]...)
			return nil
		}
	}
	return nil
}

// TransactionStore methods required by the interface.
func (f *fakeTransactionEditor) RecalculateHoldings(_ context.Context) error { return nil }
func (f *fakeTransactionEditor) UpdateMarketPrice(_ context.Context, _ string, _ float64) error {
	return nil
}
func (f *fakeTransactionEditor) ActiveSymbols(_ context.Context) ([]string, error) { return nil, nil }

// helpers ─────────────────────────────────────────────────────────────────────

func sampleTx(id int64, symbol string, kind portfolio.TransactionType) portfolio.Transaction {
	return portfolio.Transaction{
		ID:             id,
		Symbol:         symbol,
		Type:           kind,
		Quantity:       10,
		Price:          100,
		Fee:            1,
		AllocationType: portfolio.AllocationSatellite,
		ExecutedAt:     time.Date(2024, 1, int(id), 0, 0, 0, 0, time.UTC),
	}
}

// constructor ─────────────────────────────────────────────────────────────────

func TestTransactionsTabNilEditor(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(nil)
	if m.editor != nil {
		t.Error("editor must be nil")
	}
	if cmd := m.Init(); cmd != nil {
		t.Fatal("Init() with nil editor must return nil cmd")
	}
}

func TestTransactionsTabWithEditor(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	if m.Init() == nil {
		t.Fatal("Init() with non-nil editor must return a cmd")
	}
}

func TestTransactionsTabViewNilEditor(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(nil)
	view := m.View()
	if !strings.Contains(view, "not available") {
		t.Errorf("View() with nil editor = %q, want 'not available'", view)
	}
}

// buildEntries + syncRows ─────────────────────────────────────────────────────

func TestTransactionsTabBuildEntriesMergesTxsAndDivs(t *testing.T) {
	t.Parallel()

	editor := &fakeTransactionEditor{
		txs: []portfolio.Transaction{
			sampleTx(1, "VWCE.MI", portfolio.TransactionBuy),
			sampleTx(2, "ZPRV.DE", portfolio.TransactionSell),
		},
		divs: []portfolio.DividendRecord{
			{Symbol: "VWCE.MI", ExDate: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				Quantity: 5, CashDividendPerShare: 0.5, IncomeAmount: 2.5},
		},
	}
	m := NewTransactionsTabModel(editor)
	m.txs = editor.txs
	m.divs = editor.divs
	m.buildEntries()

	if len(m.entries) != 3 {
		t.Fatalf("entries len = %d, want 3", len(m.entries))
	}
	// DIV entry should be present.
	found := false
	for _, e := range m.entries {
		if e.kind == "DIV" {
			found = true
		}
	}
	if !found {
		t.Error("buildEntries() must include a DIV entry")
	}
}

func TestTransactionsTabBuildEntriesSortedNewestFirst(t *testing.T) {
	t.Parallel()

	editor := &fakeTransactionEditor{}
	m := NewTransactionsTabModel(editor)
	m.txs = []portfolio.Transaction{
		sampleTx(1, "A", portfolio.TransactionBuy), // 2024-01-01
		sampleTx(3, "B", portfolio.TransactionBuy), // 2024-01-03
		sampleTx(2, "C", portfolio.TransactionBuy), // 2024-01-02
	}
	m.buildEntries()

	if len(m.entries) < 2 {
		t.Skip("not enough entries to check order")
	}
	for i := 1; i < len(m.entries); i++ {
		if m.entries[i].date.After(m.entries[i-1].date) {
			t.Errorf("entries not sorted newest-first at index %d", i)
		}
	}
}

func TestTransactionsTabBuildEntriesBuySellSleeve(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	m.txs = []portfolio.Transaction{
		{ID: 1, Symbol: "A", Type: portfolio.TransactionBuy, AllocationType: portfolio.AllocationCore,
			Quantity: 1, Price: 1, ExecutedAt: time.Now()},
		{ID: 2, Symbol: "B", Type: portfolio.TransactionSell, AllocationType: portfolio.AllocationSatellite,
			Quantity: 1, Price: 1, ExecutedAt: time.Now()},
	}
	m.buildEntries()

	for _, e := range m.entries {
		switch e.symbol {
		case "A":
			if e.kind != "BUY" || e.sleeve != "CORE" {
				t.Errorf("A: kind=%q sleeve=%q, want BUY/CORE", e.kind, e.sleeve)
			}
		case "B":
			if e.kind != "SELL" || e.sleeve != "SAT" {
				t.Errorf("B: kind=%q sleeve=%q, want SELL/SAT", e.kind, e.sleeve)
			}
		}
	}
}

func TestTransactionsTabSyncRowsMatchesEntries(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	m.txs = []portfolio.Transaction{sampleTx(1, "AAPL", portfolio.TransactionBuy)}
	m.buildEntries()

	rows := m.tbl.Rows()
	if len(rows) != len(m.entries) {
		t.Errorf("rows len = %d, want %d", len(rows), len(m.entries))
	}
}

// knownSymbols ────────────────────────────────────────────────────────────────

func TestTransactionsTabKnownSymbolsDeduplicates(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	m.txs = []portfolio.Transaction{
		sampleTx(1, "VWCE.MI", portfolio.TransactionBuy),
		sampleTx(2, "VWCE.MI", portfolio.TransactionBuy),
		sampleTx(3, "ZPRV.DE", portfolio.TransactionBuy),
	}
	syms := m.knownSymbols()
	if len(syms) != 2 {
		t.Errorf("knownSymbols() len = %d, want 2", len(syms))
	}
}

// Update — txsLoadedMsg ───────────────────────────────────────────────────────

func TestTransactionsTabUpdateTxsLoaded(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	m.loading = true

	msg := txsLoadedMsg{
		txs: []portfolio.Transaction{sampleTx(1, "VWCE.MI", portfolio.TransactionBuy)},
	}
	updated, _ := m.Update(msg)
	mu := updated.(TransactionsTabModel)

	if mu.loading {
		t.Error("loading must be false after txsLoadedMsg")
	}
	if len(mu.txs) != 1 {
		t.Errorf("txs len = %d, want 1", len(mu.txs))
	}
}

func TestTransactionsTabUpdateTxsLoadedError(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	updated, _ := m.Update(txsLoadedMsg{err: errors.New("db down")})
	mu := updated.(TransactionsTabModel)

	if !strings.Contains(mu.status, "Load error") {
		t.Errorf("status = %q, want 'Load error'", mu.status)
	}
}

func TestTransactionsTabUpdateTxsLoadedReversesOrder(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	txs := []portfolio.Transaction{
		sampleTx(1, "A", portfolio.TransactionBuy),
		sampleTx(2, "B", portfolio.TransactionBuy),
	}
	updated, _ := m.Update(txsLoadedMsg{txs: txs})
	mu := updated.(TransactionsTabModel)

	// Transactions are reversed so newest (id=2) is first.
	if len(mu.txs) == 2 && mu.txs[0].ID != 2 {
		t.Errorf("first tx ID = %d, want 2 (newest-first)", mu.txs[0].ID)
	}
}

// Update — txAddedMsg / txUpdatedMsg / txDeletedMsg ───────────────────────────

func TestTransactionsTabUpdateTxAdded(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	updated, cmd := m.Update(txAddedMsg{})
	mu := updated.(TransactionsTabModel)

	if mu.loading {
		t.Error("loading must be false after txAddedMsg")
	}
	if cmd == nil {
		t.Error("cmd must be non-nil after txAddedMsg (triggers reload + txChangedMsg)")
	}
}

func TestTransactionsTabUpdateTxAddedError(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	updated, _ := m.Update(txAddedMsg{err: errors.New("constraint violation")})
	mu := updated.(TransactionsTabModel)

	if !strings.Contains(mu.status, "Add error") {
		t.Errorf("status = %q, want 'Add error'", mu.status)
	}
}

func TestTransactionsTabUpdateTxUpdated(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	updated, cmd := m.Update(txUpdatedMsg{})
	mu := updated.(TransactionsTabModel)

	if mu.loading {
		t.Error("loading must be false after txUpdatedMsg")
	}
	if cmd == nil {
		t.Error("cmd must be non-nil after txUpdatedMsg")
	}
}

func TestTransactionsTabUpdateTxUpdatedError(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	updated, _ := m.Update(txUpdatedMsg{err: errors.New("not found")})
	mu := updated.(TransactionsTabModel)

	if !strings.Contains(mu.status, "Update error") {
		t.Errorf("status = %q, want 'Update error'", mu.status)
	}
}

func TestTransactionsTabUpdateTxDeleted(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	updated, cmd := m.Update(txDeletedMsg{})
	mu := updated.(TransactionsTabModel)

	if mu.loading {
		t.Error("loading must be false after txDeletedMsg")
	}
	if cmd == nil {
		t.Error("cmd must be non-nil after txDeletedMsg")
	}
}

func TestTransactionsTabUpdateTxDeletedError(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	updated, _ := m.Update(txDeletedMsg{err: errors.New("foreign key")})
	mu := updated.(TransactionsTabModel)

	if !strings.Contains(mu.status, "Delete error") {
		t.Errorf("status = %q, want 'Delete error'", mu.status)
	}
}

// Update — key 'r' (reload) ───────────────────────────────────────────────────

func TestTransactionsTabKeyRReloads(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	m.loading = false

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	mu := updated.(TransactionsTabModel)

	if !mu.loading {
		t.Error("pressing 'r' must set loading=true")
	}
	if cmd == nil {
		t.Error("pressing 'r' must return a cmd")
	}
}

func TestTransactionsTabKeyRNoopWhenLoading(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	m.loading = true

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	mu := updated.(TransactionsTabModel)

	if !mu.loading {
		t.Error("loading must stay true")
	}
	if cmd != nil {
		t.Error("cmd must be nil when already loading")
	}
}

// Update — key 'a' (add) ──────────────────────────────────────────────────────

func TestTransactionsTabKeyASwitchesToAddingMode(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	mu := updated.(TransactionsTabModel)

	if mu.mode != txModeAdding {
		t.Errorf("mode = %d, want txModeAdding", mu.mode)
	}
}

func TestTransactionsTabKeyANoopWithNilEditor(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(nil)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	mu := updated.(TransactionsTabModel)

	if mu.mode != txModeNormal {
		t.Errorf("mode = %d, want normal with nil editor", mu.mode)
	}
	if cmd != nil {
		t.Error("cmd must be nil with nil editor")
	}
}

// Update — adding mode intercepts txFormResultMsg ────────────────────────────

func TestTransactionsTabAddingModeCancelReturnsToNormal(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	m.mode = txModeAdding

	updated, cmd := m.Update(txFormResultMsg{cancelled: true})
	mu := updated.(TransactionsTabModel)

	if mu.mode != txModeNormal {
		t.Errorf("mode = %d, want txModeNormal after cancel", mu.mode)
	}
	if cmd != nil {
		t.Error("cmd must be nil after cancel")
	}
}

func TestTransactionsTabAddingModeSubmitTriggersAdd(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	m.mode = txModeAdding
	tx := sampleTx(0, "AAPL", portfolio.TransactionBuy)

	updated, cmd := m.Update(txFormResultMsg{tx: &tx})
	mu := updated.(TransactionsTabModel)

	if mu.mode != txModeNormal {
		t.Errorf("mode = %d, want txModeNormal after submit", mu.mode)
	}
	if !mu.loading {
		t.Error("loading must be true while saving")
	}
	if cmd == nil {
		t.Error("cmd must be non-nil after form submit")
	}
}

// Update — editing mode ───────────────────────────────────────────────────────

func TestTransactionsTabEditingModeCancelReturnsToNormal(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	m.mode = txModeEditing

	updated, _ := m.Update(txFormResultMsg{cancelled: true})
	mu := updated.(TransactionsTabModel)

	if mu.mode != txModeNormal {
		t.Errorf("mode = %d, want txModeNormal after cancel", mu.mode)
	}
}

func TestTransactionsTabEditingModeSubmitTriggersUpdate(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	m.mode = txModeEditing
	tx := sampleTx(7, "AAPL", portfolio.TransactionBuy)

	updated, cmd := m.Update(txFormResultMsg{tx: &tx})
	mu := updated.(TransactionsTabModel)

	if mu.mode != txModeNormal {
		t.Errorf("mode = %d, want txModeNormal", mu.mode)
	}
	if cmd == nil {
		t.Error("cmd must be non-nil after edit submit")
	}
}

// Update — confirm-delete mode ────────────────────────────────────────────────

func TestTransactionsTabConfirmDeleteYConfirms(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	m.mode = txModeConfirmDelete
	m.pendingDelID = 99

	for _, key := range []string{"d", "y"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
			mu := updated.(TransactionsTabModel)
			if mu.mode != txModeNormal {
				t.Errorf("key %q: mode = %d, want normal", key, mu.mode)
			}
			if cmd == nil {
				t.Errorf("key %q: cmd must be non-nil (deleteCmd)", key)
			}
		})
	}
}

func TestTransactionsTabConfirmDeleteOtherKeyCancels(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	m.mode = txModeConfirmDelete

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	mu := updated.(TransactionsTabModel)

	if mu.mode != txModeNormal {
		t.Errorf("mode = %d, want normal after cancel key", mu.mode)
	}
	if !strings.Contains(mu.status, "cancelled") {
		t.Errorf("status = %q, want to contain 'cancelled'", mu.status)
	}
}

// Update — key 'd' (delete) ───────────────────────────────────────────────────

func TestTransactionsTabKeyDOnDivRow(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	m.divs = []portfolio.DividendRecord{
		{Symbol: "VWCE.MI", ExDate: time.Now(), Quantity: 1, CashDividendPerShare: 0.5, IncomeAmount: 0.5},
	}
	m.buildEntries()
	// Make the DIV row the selected one by rebuilding after sort.
	// Find the DIV entry index.
	for i, e := range m.entries {
		if e.kind == "DIV" {
			m.tbl.SetCursor(i)
			break
		}
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mu := updated.(TransactionsTabModel)

	if mu.mode == txModeConfirmDelete {
		t.Error("pressing 'd' on a DIV row must NOT enter confirm-delete mode")
	}
}

// Update — key 'e' (edit) ─────────────────────────────────────────────────────

func TestTransactionsTabKeyEOnDivRow(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	m.divs = []portfolio.DividendRecord{
		{Symbol: "VWCE.MI", ExDate: time.Now(), Quantity: 1, CashDividendPerShare: 0.5, IncomeAmount: 0.5},
	}
	m.buildEntries()
	for i, e := range m.entries {
		if e.kind == "DIV" {
			m.tbl.SetCursor(i)
			break
		}
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	mu := updated.(TransactionsTabModel)

	if mu.mode == txModeEditing {
		t.Error("pressing 'e' on a DIV row must NOT enter edit mode")
	}
}

func TestTransactionsTabKeyEOnTxRowEntersEditMode(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	m.txs = []portfolio.Transaction{sampleTx(1, "AAPL", portfolio.TransactionBuy)}
	m.buildEntries()
	m.tbl.SetCursor(0)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	mu := updated.(TransactionsTabModel)

	if mu.mode != txModeEditing {
		t.Errorf("mode = %d, want txModeEditing", mu.mode)
	}
	if cmd == nil {
		t.Error("cmd must be non-nil (text input blink cmd)")
	}
}

// View ────────────────────────────────────────────────────────────────────────

func TestTransactionsTabViewNormalMode(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	m.status = "ready"
	// Status is surfaced via NavHint() (merged into the root help line),
	// not embedded in View() — the body renders only the table.
	if !strings.Contains(m.NavHint(), "ready") {
		t.Errorf("NavHint() = %q, want status 'ready'", m.NavHint())
	}
}

func TestTransactionsTabViewAddingMode(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	m.mode = txModeAdding
	var cmd tea.Cmd
	m.form, cmd = newTransactionForm(nil)
	_ = cmd
	view := m.View()
	if !strings.Contains(view, "Add Transaction") {
		t.Errorf("View() in adding mode = %q, want 'Add Transaction'", view)
	}
}

// WindowSizeMsg ───────────────────────────────────────────────────────────────

func TestTransactionsTabWindowSizeMsg(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	_ = updated.(TransactionsTabModel)
	if cmd != nil {
		t.Errorf("WindowSizeMsg cmd = non-nil, want nil")
	}
}

// status string format ────────────────────────────────────────────────────────

func TestTransactionsTabStatusAfterLoad(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	txs := []portfolio.Transaction{
		sampleTx(1, "A", portfolio.TransactionBuy),
		sampleTx(2, "B", portfolio.TransactionSell),
	}
	divs := []portfolio.DividendRecord{
		{Symbol: "A", ExDate: time.Now(), Quantity: 1, CashDividendPerShare: 0.1, IncomeAmount: 0.1},
	}
	updated, _ := m.Update(txsLoadedMsg{txs: txs, divs: divs})
	mu := updated.(TransactionsTabModel)

	if !strings.Contains(mu.status, "2 transactions") {
		t.Errorf("status = %q, want '2 transactions'", mu.status)
	}
	if !strings.Contains(mu.status, "1 dividends") {
		t.Errorf("status = %q, want '1 dividends'", mu.status)
	}
}

// key 'd' triggers confirm-delete for a real tx row ───────────────────────────

func TestTransactionsTabKeyDOnTxRowEntersConfirmMode(t *testing.T) {
	t.Parallel()

	m := NewTransactionsTabModel(&fakeTransactionEditor{})
	tx := sampleTx(5, "AAPL", portfolio.TransactionBuy)
	m.txs = []portfolio.Transaction{tx}
	m.buildEntries()
	m.tbl.SetCursor(0)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mu := updated.(TransactionsTabModel)

	if mu.mode != txModeConfirmDelete {
		t.Errorf("mode = %d, want txModeConfirmDelete", mu.mode)
	}
	if mu.pendingDelID != 5 {
		t.Errorf("pendingDelID = %d, want 5", mu.pendingDelID)
	}
	if !strings.Contains(mu.status, tx.Symbol) {
		t.Errorf("status = %q, want to mention symbol %q", mu.status, tx.Symbol)
	}
}
