package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tgragnato/orbiter/internal/portfolio"
)

const (
	formFieldSymbol = iota
	formFieldType   // BUY / SELL toggle
	formFieldQty
	formFieldPrice
	formFieldFee
	formFieldDate  // YYYY-MM-DD execution date
	formFieldAlloc // CORE / SATELLITE toggle
	formFieldCount = 7
)

// txFormResultMsg is produced by the form when the user confirms or cancels.
type txFormResultMsg struct {
	tx        *portfolio.Transaction // nil when cancelled
	cancelled bool
}

// transactionFormModel is a self-contained BubbleTea sub-model for entering
// a single trade. It is embedded in the holdings Model and rendered as an
// overlay when mode == modeAddTx. When txID != 0, the form is in edit mode.
type transactionFormModel struct {
	symbolInput      textinput.Model
	qtyInput         textinput.Model
	priceInput       textinput.Model
	feeInput         textinput.Model
	dateInput        textinput.Model
	txType           portfolio.TransactionType
	allocType        portfolio.AllocationType
	focused          int
	errMsg           string
	knownSymbols     []string
	autocompleteHint string
	txID             int64 // non-zero when editing an existing transaction

	formStyles formStyles
}

type formStyles struct {
	title      lipgloss.Style
	divider    lipgloss.Style
	label      lipgloss.Style
	focused    lipgloss.Style
	unfocused  lipgloss.Style
	toggleOn   lipgloss.Style
	toggleOff  lipgloss.Style
	errorStyle lipgloss.Style
	hint       lipgloss.Style
}

func newFormStyles() formStyles {
	return formStyles{
		title:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33")),
		divider:    lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		label:      lipgloss.NewStyle().Width(14).Foreground(lipgloss.Color("252")),
		focused:    lipgloss.NewStyle().Foreground(lipgloss.Color("33")),
		unfocused:  lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
		toggleOn:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("28")).Padding(0, 1),
		toggleOff:  lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Padding(0, 1),
		errorStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		hint:       lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
	}
}

// newTransactionForm initialises a fresh form with default values and returns
// the initial blink command for the first text input.
func newTransactionForm(knownSymbols []string) (transactionFormModel, tea.Cmd) {
	sym := textinput.New()
	sym.Placeholder = "e.g. VWCE.MI"
	sym.CharLimit = 24
	sym.Width = 22

	qty := textinput.New()
	qty.Placeholder = "e.g. 100"
	qty.CharLimit = 16
	qty.Width = 16

	price := textinput.New()
	price.Placeholder = "e.g. 98.50"
	price.CharLimit = 14
	price.Width = 14

	fee := textinput.New()
	fee.Placeholder = "0"
	fee.CharLimit = 12
	fee.Width = 12
	fee.SetValue("0")

	date := textinput.New()
	date.Placeholder = "YYYY-MM-DD"
	date.CharLimit = 10
	date.Width = 12
	date.SetValue(time.Now().Format("2006-01-02"))

	f := transactionFormModel{
		symbolInput:  sym,
		qtyInput:     qty,
		priceInput:   price,
		feeInput:     fee,
		dateInput:    date,
		txType:       portfolio.TransactionBuy,
		allocType:    portfolio.AllocationSatellite,
		focused:      formFieldSymbol,
		formStyles:   newFormStyles(),
		knownSymbols: knownSymbols,
	}
	cmd := f.symbolInput.Focus()
	return f, cmd
}

// withUpdatedAutocomplete returns a copy of f with the autocompleteHint
// recalculated from the current symbol input value.
func (f transactionFormModel) withUpdatedAutocomplete() transactionFormModel {
	prefix := strings.ToUpper(strings.TrimSpace(f.symbolInput.Value()))
	f.autocompleteHint = ""
	if prefix == "" || len(f.knownSymbols) == 0 {
		return f
	}
	for _, sym := range f.knownSymbols {
		if strings.HasPrefix(sym, prefix) && sym != prefix {
			f.autocompleteHint = sym
			return f
		}
	}
	return f
}

func (f transactionFormModel) jumpToQty() (transactionFormModel, tea.Cmd) {
	f.blurCurrent()
	f.focused = formFieldQty
	cmd := f.focusCurrent()
	return f, cmd
}

func (f transactionFormModel) Update(msg tea.Msg) (transactionFormModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			if f.focused == formFieldSymbol && f.autocompleteHint != "" {
				f.autocompleteHint = ""
				return f, nil
			}
			return f, func() tea.Msg { return txFormResultMsg{cancelled: true} }

		case "enter":
			if f.focused == formFieldSymbol && f.autocompleteHint != "" {
				f.symbolInput.SetValue(f.autocompleteHint)
				f.autocompleteHint = ""
				return f.jumpToQty()
			}
			tx, errMsg := f.buildTx()
			if errMsg != "" {
				f.errMsg = errMsg
				return f, nil
			}
			return f, func() tea.Msg { return txFormResultMsg{tx: &tx} }

		case "tab":
			if f.focused == formFieldSymbol && f.autocompleteHint != "" {
				f.symbolInput.SetValue(f.autocompleteHint)
				f.autocompleteHint = ""
				return f.jumpToQty()
			}
			return f.moveFocus(1)

		case "shift+tab":
			if f.focused == formFieldSymbol {
				f.autocompleteHint = ""
			}
			return f.moveFocus(-1)

		case "down":
			if f.focused == formFieldSymbol && f.autocompleteHint != "" {
				f.symbolInput.SetValue(f.autocompleteHint)
				f.autocompleteHint = ""
				return f, nil
			}

		case " ":
			switch f.focused {
			case formFieldType:
				if f.txType == portfolio.TransactionBuy {
					f.txType = portfolio.TransactionSell
				} else {
					f.txType = portfolio.TransactionBuy
				}
				return f, nil
			case formFieldAlloc:
				if f.allocType == portfolio.AllocationCore {
					f.allocType = portfolio.AllocationSatellite
				} else {
					f.allocType = portfolio.AllocationCore
				}
				return f, nil
			}
			// For text inputs, fall through to updateFocusedInput below.
		}
	}

	result, cmd := f.updateFocusedInput(msg)
	if result.focused == formFieldSymbol {
		result = result.withUpdatedAutocomplete()
	}
	return result, cmd
}

func (f transactionFormModel) moveFocus(delta int) (transactionFormModel, tea.Cmd) {
	f.blurCurrent()
	f.focused = ((f.focused+delta)%formFieldCount + formFieldCount) % formFieldCount
	cmd := f.focusCurrent()
	return f, cmd
}

func (f *transactionFormModel) blurCurrent() {
	switch f.focused {
	case formFieldSymbol:
		f.symbolInput.Blur()
	case formFieldQty:
		f.qtyInput.Blur()
	case formFieldPrice:
		f.priceInput.Blur()
	case formFieldFee:
		f.feeInput.Blur()
	case formFieldDate:
		f.dateInput.Blur()
	}
}

func (f *transactionFormModel) focusCurrent() tea.Cmd {
	switch f.focused {
	case formFieldSymbol:
		return f.symbolInput.Focus()
	case formFieldQty:
		return f.qtyInput.Focus()
	case formFieldPrice:
		return f.priceInput.Focus()
	case formFieldFee:
		return f.feeInput.Focus()
	case formFieldDate:
		return f.dateInput.Focus()
	}
	return nil
}

func (f transactionFormModel) updateFocusedInput(msg tea.Msg) (transactionFormModel, tea.Cmd) {
	var cmd tea.Cmd
	switch f.focused {
	case formFieldSymbol:
		f.symbolInput, cmd = f.symbolInput.Update(msg)
	case formFieldQty:
		f.qtyInput, cmd = f.qtyInput.Update(msg)
	case formFieldPrice:
		f.priceInput, cmd = f.priceInput.Update(msg)
	case formFieldFee:
		f.feeInput, cmd = f.feeInput.Update(msg)
	case formFieldDate:
		f.dateInput, cmd = f.dateInput.Update(msg)
	}
	return f, cmd
}

// buildTx validates form inputs and returns a ready-to-persist Transaction or
// an error message string if validation fails.
func (f transactionFormModel) buildTx() (tx portfolio.Transaction, errMsg string) {
	symbol := strings.ToUpper(strings.TrimSpace(f.symbolInput.Value()))
	if symbol == "" {
		return portfolio.Transaction{}, "symbol is required"
	}

	qty, err := strconv.ParseFloat(strings.TrimSpace(f.qtyInput.Value()), 64)
	if err != nil || qty <= 0 {
		return portfolio.Transaction{}, "quantity must be a positive number"
	}

	price, err := strconv.ParseFloat(strings.TrimSpace(f.priceInput.Value()), 64)
	if err != nil || price <= 0 {
		return portfolio.Transaction{}, "price must be a positive number"
	}

	feeStr := strings.TrimSpace(f.feeInput.Value())
	if feeStr == "" {
		feeStr = "0"
	}
	fee, err := strconv.ParseFloat(feeStr, 64)
	if err != nil || fee < 0 {
		return portfolio.Transaction{}, "fee must be >= 0"
	}

	dateStr := strings.TrimSpace(f.dateInput.Value())
	var executedAt time.Time
	if dateStr == "" {
		executedAt = time.Now().UTC()
	} else {
		parsed, parseErr := time.Parse("2006-01-02", dateStr)
		if parseErr != nil {
			return portfolio.Transaction{}, "date must be YYYY-MM-DD"
		}
		executedAt = parsed.UTC()
	}

	return portfolio.Transaction{
		ID:             f.txID,
		Symbol:         symbol,
		Type:           f.txType,
		Quantity:       qty,
		Price:          price,
		Fee:            fee,
		AllocationType: f.allocType,
		ExecutedAt:     executedAt,
	}, ""
}

// newTransactionFormEditing creates a form pre-filled with the values of an
// existing transaction for editing. On submit, txFormResultMsg.tx.ID is set.
func newTransactionFormEditing(tx portfolio.Transaction, knownSymbols []string) (transactionFormModel, tea.Cmd) {
	f, cmd := newTransactionForm(knownSymbols)
	f.txID = tx.ID
	f.txType = tx.Type
	f.allocType = tx.AllocationType
	f.symbolInput.SetValue(tx.Symbol)
	f.qtyInput.SetValue(fmt.Sprintf("%g", tx.Quantity))
	f.priceInput.SetValue(fmt.Sprintf("%g", tx.Price))
	f.feeInput.SetValue(fmt.Sprintf("%g", tx.Fee))
	f.dateInput.SetValue(tx.ExecutedAt.Format("2006-01-02"))
	return f, cmd
}

func (f transactionFormModel) View() string {
	st := f.formStyles
	divider := st.divider.Render(strings.Repeat("─", 44))

	title := "Add Transaction"
	if f.txID != 0 {
		title = "Edit Transaction"
	}

	lines := []string{
		st.title.Render(title),
		divider,
		f.renderInput("Symbol:      ", f.symbolInput, f.focused == formFieldSymbol),
	}
	if f.focused == formFieldSymbol && f.autocompleteHint != "" {
		lines = append(lines, st.hint.Render("  → "+f.autocompleteHint+" (↓/tab: accept)"))
	}
	lines = append(lines,
		f.renderToggle("Type:        ", "BUY", "SELL",
			f.txType == portfolio.TransactionBuy, f.focused == formFieldType),
		f.renderInput("Quantity:    ", f.qtyInput, f.focused == formFieldQty),
		f.renderInput("Price:       ", f.priceInput, f.focused == formFieldPrice),
		f.renderInput("Fee:         ", f.feeInput, f.focused == formFieldFee),
		f.renderInput("Date:        ", f.dateInput, f.focused == formFieldDate),
		f.renderToggle("Allocation:  ", "CORE", "SAT",
			f.allocType == portfolio.AllocationCore, f.focused == formFieldAlloc),
		"",
	)

	if f.errMsg != "" {
		lines = append(lines, st.errorStyle.Render(fmt.Sprintf("  Error: %s", f.errMsg)))
	}
	lines = append(lines, st.hint.Render("  tab: next · space: toggle · enter: confirm · esc: cancel"))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (f transactionFormModel) renderInput(label string, inp textinput.Model, isFocused bool) string {
	lStyle := f.formStyles.label
	if isFocused {
		lStyle = lStyle.Foreground(lipgloss.Color("33"))
	}
	return lStyle.Render(label) + inp.View()
}

func (f transactionFormModel) renderToggle(label, optA, optB string, aSelected, isFocused bool) string {
	st := f.formStyles
	lStyle := st.label
	if isFocused {
		lStyle = lStyle.Foreground(lipgloss.Color("33"))
	}

	var rendA, rendB string
	if aSelected {
		rendA = st.toggleOn.Render(optA)
		rendB = st.toggleOff.Render(optB)
	} else {
		rendA = st.toggleOff.Render(optA)
		rendB = st.toggleOn.Render(optB)
	}
	focusHint := ""
	if isFocused {
		focusHint = st.hint.Render(" (space: toggle)")
	}
	return lStyle.Render(label) + rendA + " " + rendB + focusHint
}
