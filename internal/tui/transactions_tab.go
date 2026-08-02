package tui

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tgragnato/orbiter/internal/portfolio"
)

// TransactionEditor extends TransactionStore with edit, delete, and dividend
// listing capabilities.
type TransactionEditor interface {
	portfolio.TransactionStore
	UpdateTransaction(ctx context.Context, tx portfolio.Transaction) error
	DeleteTransaction(ctx context.Context, id int64) error
	ListDividendIncome(ctx context.Context) ([]portfolio.DividendRecord, error)
}

// txsLoadedMsg carries the result of loading all transactions and dividends.
type txsLoadedMsg struct {
	txs  []portfolio.Transaction
	divs []portfolio.DividendRecord
	err  error
}

// txAddedMsg carries the result of an AddTransaction call.
type txAddedMsg struct{ err error }

// txUpdatedMsg carries the result of an UpdateTransaction call.
type txUpdatedMsg struct{ err error }

// txDeletedMsg carries the result of a DeleteTransaction call.
type txDeletedMsg struct{ err error }

// txChangedMsg signals the root model to refresh the holdings tab.
type txChangedMsg struct{}

const (
	txModeNormal = iota
	txModeAdding
	txModeEditing
	txModeConfirmDelete
)

// txDisplayEntry is a unified row for the transactions table — either a
// BUY/SELL trade or a DIV (dividend) income record.
type txDisplayEntry struct {
	date    time.Time
	symbol  string
	kind    string  // "BUY", "SELL", "DIV"
	qty     float64
	price   float64 // trade price / per-share dividend
	fee     float64 // trade fee / total income for DIV
	sleeve  string
	txIndex int // index into m.txs; -1 for dividend rows
}

// TransactionsTabModel is the Tab 5 sub-model for viewing, editing, and
// deleting individual trade records and viewing dividend income.
type TransactionsTabModel struct {
	editor  TransactionEditor
	txs     []portfolio.Transaction
	divs    []portfolio.DividendRecord
	entries []txDisplayEntry
	tbl     table.Model
	status  string
	loading bool
	mode    int
	form    transactionFormModel
	pendingDelID int64
}

func NewTransactionsTabModel(editor TransactionEditor) TransactionsTabModel {
	cols := []table.Column{
		{Title: "Symbol", Width: 10},
		{Title: "Date", Width: 12},
		{Title: "Type", Width: 5},
		{Title: "Qty", Width: 10},
		{Title: "Price", Width: 10},
		{Title: "Fee/Inc", Width: 9},
		{Title: "Sleeve", Width: 8},
	}
	tbl := table.New(
		table.WithColumns(cols),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(20),
	)
	tbl.KeyMap.LineUp.SetKeys("up", "k")
	tbl.KeyMap.LineDown.SetKeys("down", "j")

	status := "No transaction store configured"
	if editor != nil {
		status = "Loading..."
	}

	return TransactionsTabModel{
		editor: editor,
		tbl:    tbl,
		status: status,
	}
}

func (m TransactionsTabModel) Init() tea.Cmd {
	if m.editor == nil {
		return nil
	}
	return m.loadCmd()
}

func (m TransactionsTabModel) loadCmd() tea.Cmd {
	return func() tea.Msg {
		txs, err := m.editor.ListTransactions(context.Background(), "")
		if err != nil {
			return txsLoadedMsg{err: err}
		}
		divs, _ := m.editor.ListDividendIncome(context.Background())
		return txsLoadedMsg{txs: txs, divs: divs}
	}
}

func (m TransactionsTabModel) addCmd(tx portfolio.Transaction) tea.Cmd {
	return func() tea.Msg {
		return txAddedMsg{err: m.editor.AddTransaction(context.Background(), tx)}
	}
}

func (m TransactionsTabModel) updateCmd(tx portfolio.Transaction) tea.Cmd {
	return func() tea.Msg {
		return txUpdatedMsg{err: m.editor.UpdateTransaction(context.Background(), tx)}
	}
}

func (m TransactionsTabModel) deleteCmd(id int64) tea.Cmd {
	return func() tea.Msg {
		return txDeletedMsg{err: m.editor.DeleteTransaction(context.Background(), id)}
	}
}

func (m TransactionsTabModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Add form intercepts all messages while active.
	if m.mode == txModeAdding {
		switch msg := msg.(type) {
		case txFormResultMsg:
			if msg.cancelled {
				m.mode = txModeNormal
				m.status = "Add cancelled"
				return m, nil
			}
			m.mode = txModeNormal
			m.loading = true
			m.status = "Saving..."
			return m, m.addCmd(*msg.tx)
		default:
			var cmd tea.Cmd
			m.form, cmd = m.form.Update(msg)
			return m, cmd
		}
	}

	// Edit form intercepts all messages while active.
	if m.mode == txModeEditing {
		switch msg := msg.(type) {
		case txFormResultMsg:
			if msg.cancelled {
				m.mode = txModeNormal
				m.status = "Edit cancelled"
				return m, nil
			}
			m.mode = txModeNormal
			m.loading = true
			m.status = "Saving..."
			return m, m.updateCmd(*msg.tx)
		default:
			var cmd tea.Cmd
			m.form, cmd = m.form.Update(msg)
			return m, cmd
		}
	}

	// Confirm-delete mode: d/y confirms, any other key cancels.
	if m.mode == txModeConfirmDelete {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "d", "y":
				m.mode = txModeNormal
				m.loading = true
				m.status = "Deleting..."
				return m, m.deleteCmd(m.pendingDelID)
			default:
				m.mode = txModeNormal
				m.status = "Delete cancelled"
			}
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "a":
			if m.editor == nil {
				return m, nil
			}
			m.mode = txModeAdding
			var cmd tea.Cmd
			m.form, cmd = newTransactionForm(m.knownSymbols())
			return m, cmd

		case "e":
			if m.editor == nil || len(m.entries) == 0 {
				return m, nil
			}
			cur := m.tbl.Cursor()
			if cur < 0 || cur >= len(m.entries) {
				return m, nil
			}
			entry := m.entries[cur]
			if entry.txIndex < 0 {
				m.status = "Dividend records cannot be edited — delete and re-add the transaction to adjust"
				return m, nil
			}
			m.mode = txModeEditing
			var cmd tea.Cmd
			m.form, cmd = newTransactionFormEditing(m.txs[entry.txIndex], m.knownSymbols())
			return m, cmd

		case "d":
			if m.editor == nil || len(m.entries) == 0 {
				return m, nil
			}
			cur := m.tbl.Cursor()
			if cur < 0 || cur >= len(m.entries) {
				return m, nil
			}
			entry := m.entries[cur]
			if entry.txIndex < 0 {
				m.status = "Dividend records are computed automatically and cannot be deleted"
				return m, nil
			}
			tx := m.txs[entry.txIndex]
			m.pendingDelID = tx.ID
			m.mode = txModeConfirmDelete
			m.status = fmt.Sprintf("Delete %s %s %.4f @ %.2f? d/y: confirm · any other key: cancel",
				tx.Symbol, tx.Type, tx.Quantity, tx.Price)
			return m, nil

		case "r":
			if m.loading || m.editor == nil {
				return m, nil
			}
			m.loading = true
			m.status = "Loading..."
			return m, m.loadCmd()
		}

	case tea.WindowSizeMsg:
		m.tbl.SetWidth(max(40, msg.Width-4))
		m.tbl.SetHeight(max(10, msg.Height-10))
		return m, nil

	case txsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.status = fmt.Sprintf("Load error: %v", msg.err)
			return m, nil
		}
		// Reverse transactions so newest appears first.
		for i, j := 0, len(msg.txs)-1; i < j; i, j = i+1, j-1 {
			msg.txs[i], msg.txs[j] = msg.txs[j], msg.txs[i]
		}
		m.txs = msg.txs
		m.divs = msg.divs
		m.buildEntries()
		nDivs := len(msg.divs)
		m.status = fmt.Sprintf("%d transactions · %d dividends | a: add · e: edit · d: delete · r: reload",
			len(msg.txs), nDivs)
		return m, nil

	case txAddedMsg:
		m.loading = false
		if msg.err != nil {
			m.status = fmt.Sprintf("Add error: %v", msg.err)
			return m, nil
		}
		m.status = "Transaction added — reloading..."
		return m, tea.Batch(m.loadCmd(), func() tea.Msg { return txChangedMsg{} })

	case txUpdatedMsg:
		m.loading = false
		if msg.err != nil {
			m.status = fmt.Sprintf("Update error: %v", msg.err)
			return m, nil
		}
		m.status = "Transaction updated — reloading..."
		return m, tea.Batch(m.loadCmd(), func() tea.Msg { return txChangedMsg{} })

	case txDeletedMsg:
		m.loading = false
		if msg.err != nil {
			m.status = fmt.Sprintf("Delete error: %v", msg.err)
			return m, nil
		}
		m.status = "Transaction deleted — reloading..."
		return m, tea.Batch(m.loadCmd(), func() tea.Msg { return txChangedMsg{} })
	}

	var cmd tea.Cmd
	m.tbl, cmd = m.tbl.Update(msg)
	return m, cmd
}

func (m TransactionsTabModel) View() string {
	if m.editor == nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render(
			"Transaction editor not available — no transaction store configured.",
		)
	}

	if m.mode == txModeAdding || m.mode == txModeEditing {
		return m.form.View()
	}

	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	return lipgloss.JoinVertical(lipgloss.Left,
		m.tbl.View(),
		statusStyle.Render(m.status),
	)
}

// buildEntries merges m.txs and m.divs into a single date-sorted slice and
// refreshes the table rows. Transactions in m.txs must already be newest-first.
func (m *TransactionsTabModel) buildEntries() {
	entries := make([]txDisplayEntry, 0, len(m.txs)+len(m.divs))

	for i, tx := range m.txs {
		kind := "BUY"
		if tx.Type == portfolio.TransactionSell {
			kind = "SELL"
		}
		sleeve := "SAT"
		if tx.AllocationType == portfolio.AllocationCore {
			sleeve = "CORE"
		}
		entries = append(entries, txDisplayEntry{
			date: tx.ExecutedAt, symbol: tx.Symbol, kind: kind,
			qty: tx.Quantity, price: tx.Price, fee: tx.Fee,
			sleeve: sleeve, txIndex: i,
		})
	}

	for _, d := range m.divs {
		entries = append(entries, txDisplayEntry{
			date: d.ExDate, symbol: d.Symbol, kind: "DIV",
			qty: d.Quantity, price: d.CashDividendPerShare, fee: d.IncomeAmount,
			sleeve: "—", txIndex: -1,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].date.After(entries[j].date)
	})

	m.entries = entries
	m.syncRows()
}

func (m *TransactionsTabModel) syncRows() {
	rows := make([]table.Row, 0, len(m.entries))
	for _, e := range m.entries {
		rows = append(rows, table.Row{
			e.symbol,
			e.date.Format("2006-01-02"),
			e.kind,
			fmt.Sprintf("%.4f", e.qty),
			fmt.Sprintf("%.2f", e.price),
			fmt.Sprintf("%.2f", e.fee),
			e.sleeve,
		})
	}
	m.tbl.SetRows(rows)
}

func (m TransactionsTabModel) knownSymbols() []string {
	seen := make(map[string]bool)
	var syms []string
	for _, tx := range m.txs {
		if !seen[tx.Symbol] {
			seen[tx.Symbol] = true
			syms = append(syms, tx.Symbol)
		}
	}
	return syms
}
