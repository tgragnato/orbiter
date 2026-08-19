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
)

const formFieldCount = 7

const (
	formLabelWidth = 14
	formDividerLen = 44
)

// txFormResultMsg is produced by the form when the user confirms or cancels.
type txFormResultMsg struct {
	tx        *portfolio.Transaction // nil when cancelled
	cancelled bool
}

// transactionFormModel is a self-contained BubbleTea sub-model for entering
// a single trade. It is embedded in the holdings Model and rendered as an
// overlay when mode == modeAddTx. When txID != 0, the form is in edit mode.
//
//nolint:recvcheck // tea.Model interface requires value receivers; blurCurrent/focusCurrent use pointer
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
		title:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33")),
		divider:  lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		label:    lipgloss.NewStyle().Width(formLabelWidth).Foreground(lipgloss.Color("252")),
		focused:  lipgloss.NewStyle().Foreground(lipgloss.Color("33")),
		unfocused: lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
		//nolint:lll // style chain is naturally long; splitting across lines hurts readability
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

	form := transactionFormModel{
		symbolInput:      sym,
		qtyInput:         qty,
		priceInput:       price,
		feeInput:         fee,
		dateInput:        date,
		txType:           portfolio.TransactionBuy,
		allocType:        portfolio.AllocationSatellite,
		focused:          formFieldSymbol,
		errMsg:           "",
		autocompleteHint: "",
		txID:             0,
		formStyles:       newFormStyles(),
		knownSymbols:     knownSymbols,
	}
	cmd := form.symbolInput.Focus()

	return form, cmd
}

// newTransactionFormEditing creates a form pre-filled with the values of an
// existing transaction for editing. On submit, txFormResultMsg.tx.ID is set.
func newTransactionFormEditing(
	transaction portfolio.Transaction, knownSymbols []string,
) (transactionFormModel, tea.Cmd) {
	form, cmd := newTransactionForm(knownSymbols)
	form.txID = transaction.ID
	form.txType = transaction.Type
	form.allocType = transaction.AllocationType
	form.symbolInput.SetValue(transaction.Symbol)
	form.qtyInput.SetValue(fmt.Sprintf("%g", transaction.Quantity))
	form.priceInput.SetValue(fmt.Sprintf("%g", transaction.Price))
	form.feeInput.SetValue(fmt.Sprintf("%g", transaction.Fee))
	form.dateInput.SetValue(transaction.ExecutedAt.Format("2006-01-02"))

	return form, cmd
}

// Update handles incoming messages for the transaction form.
//
//nolint:gocognit,cyclop,funlen // form logic has inherent branches for each field + action; extracting adds no clarity
func (form transactionFormModel) Update(msg tea.Msg) (transactionFormModel, tea.Cmd) {
	//nolint:nestif // complex autocomplete + form-field dispatch; all branches required
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			if form.focused == formFieldSymbol && form.autocompleteHint != "" {
				form.autocompleteHint = ""

				return form, nil
			}

			return form, func() tea.Msg { return txFormResultMsg{cancelled: true, tx: nil} }

		case "enter":
			if form.focused == formFieldSymbol && form.autocompleteHint != "" {
				form.symbolInput.SetValue(form.autocompleteHint)
				form.autocompleteHint = ""

				return form.jumpToQty()
			}

			builtTx, errMsg := form.buildTx()
			if errMsg != "" {
				form.errMsg = errMsg

				return form, nil
			}

			return form, func() tea.Msg { return txFormResultMsg{tx: &builtTx, cancelled: false} }

		case "tab":
			if form.focused == formFieldSymbol && form.autocompleteHint != "" {
				form.symbolInput.SetValue(form.autocompleteHint)
				form.autocompleteHint = ""

				return form.jumpToQty()
			}

			return form.moveFocus(1)

		case "shift+tab":
			if form.focused == formFieldSymbol {
				form.autocompleteHint = ""
			}

			return form.moveFocus(-1)

		case keyDown:
			if form.focused == formFieldSymbol && form.autocompleteHint != "" {
				form.symbolInput.SetValue(form.autocompleteHint)
				form.autocompleteHint = ""

				return form, nil
			}

		case " ":
			switch form.focused {
			case formFieldType:
				if form.txType == portfolio.TransactionBuy {
					form.txType = portfolio.TransactionSell
				} else {
					form.txType = portfolio.TransactionBuy
				}

				return form, nil
			case formFieldAlloc:
				if form.allocType == portfolio.AllocationCore {
					form.allocType = portfolio.AllocationSatellite
				} else {
					form.allocType = portfolio.AllocationCore
				}

				return form, nil
			}
			// For text inputs, fall through to updateFocusedInput below.
		}
	}

	result, cmd := form.updateFocusedInput(msg)
	if result.focused == formFieldSymbol {
		result = result.withUpdatedAutocomplete()
	}

	return result, cmd
}

// View renders the transaction form overlay.
func (form transactionFormModel) View() string {
	styles := form.formStyles
	divider := styles.divider.Render(strings.Repeat("─", formDividerLen))

	title := "Add Transaction"
	if form.txID != 0 {
		title = "Edit Transaction"
	}

	lines := []string{
		styles.title.Render(title),
		divider,
		form.renderInput("Symbol:      ", form.symbolInput, form.focused == formFieldSymbol),
	}
	if form.focused == formFieldSymbol && form.autocompleteHint != "" {
		lines = append(lines, styles.hint.Render("  → "+form.autocompleteHint+" (↓/tab: accept)"))
	}

	lines = append(lines,
		form.renderToggle("Type:        ", "BUY", "SELL",
			form.txType == portfolio.TransactionBuy, form.focused == formFieldType),
		form.renderInput("Quantity:    ", form.qtyInput, form.focused == formFieldQty),
		form.renderInput("Price:       ", form.priceInput, form.focused == formFieldPrice),
		form.renderInput("Fee:         ", form.feeInput, form.focused == formFieldFee),
		form.renderInput("Date:        ", form.dateInput, form.focused == formFieldDate),
		form.renderToggle("Allocation:  ", "CORE", "SAT",
			form.allocType == portfolio.AllocationCore, form.focused == formFieldAlloc),
		"",
	)

	if form.errMsg != "" {
		lines = append(lines, styles.errorStyle.Render("  Error: "+form.errMsg))
	}

	lines = append(lines, styles.hint.Render("  tab: next · space: toggle · enter: confirm · esc: cancel"))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// withUpdatedAutocomplete returns a copy of form with the autocompleteHint
// recalculated from the current symbol input value.
func (form transactionFormModel) withUpdatedAutocomplete() transactionFormModel {
	prefix := strings.ToUpper(strings.TrimSpace(form.symbolInput.Value()))

	form.autocompleteHint = ""
	if prefix == "" || len(form.knownSymbols) == 0 {
		return form
	}

	for _, sym := range form.knownSymbols {
		if strings.HasPrefix(sym, prefix) && sym != prefix {
			form.autocompleteHint = sym

			return form
		}
	}

	return form
}

func (form transactionFormModel) jumpToQty() (transactionFormModel, tea.Cmd) {
	form.blurCurrent()
	form.focused = formFieldQty
	cmd := form.focusCurrent()

	return form, cmd
}

func (form transactionFormModel) moveFocus(delta int) (transactionFormModel, tea.Cmd) {
	form.blurCurrent()
	form.focused = ((form.focused+delta)%formFieldCount + formFieldCount) % formFieldCount
	cmd := form.focusCurrent()

	return form, cmd
}

func (form *transactionFormModel) blurCurrent() {
	switch form.focused {
	case formFieldSymbol:
		form.symbolInput.Blur()
	case formFieldQty:
		form.qtyInput.Blur()
	case formFieldPrice:
		form.priceInput.Blur()
	case formFieldFee:
		form.feeInput.Blur()
	case formFieldDate:
		form.dateInput.Blur()
	}
}

func (form *transactionFormModel) focusCurrent() tea.Cmd {
	switch form.focused {
	case formFieldSymbol:
		return form.symbolInput.Focus()
	case formFieldQty:
		return form.qtyInput.Focus()
	case formFieldPrice:
		return form.priceInput.Focus()
	case formFieldFee:
		return form.feeInput.Focus()
	case formFieldDate:
		return form.dateInput.Focus()
	}

	return nil
}

func (form transactionFormModel) updateFocusedInput(msg tea.Msg) (transactionFormModel, tea.Cmd) {
	var cmd tea.Cmd

	switch form.focused {
	case formFieldSymbol:
		form.symbolInput, cmd = form.symbolInput.Update(msg)
	case formFieldQty:
		form.qtyInput, cmd = form.qtyInput.Update(msg)
	case formFieldPrice:
		form.priceInput, cmd = form.priceInput.Update(msg)
	case formFieldFee:
		form.feeInput, cmd = form.feeInput.Update(msg)
	case formFieldDate:
		form.dateInput, cmd = form.dateInput.Update(msg)
	}

	return form, cmd
}

// buildTx validates form inputs and returns a ready-to-persist Transaction or
// an error message string if validation fails.
//
//nolint:cyclop,funlen // validation necessarily checks each field independently; splitting adds no value
func (form transactionFormModel) buildTx() (portfolio.Transaction, string) {
	emptyTx := portfolio.Transaction{
		ID:             0,
		Symbol:         "",
		Type:           "",
		Quantity:       0,
		Price:          0,
		Fee:            0,
		AllocationType: "",
		Currency:       "",
		ExecutedAt:     time.Time{},
		CreatedAt:      time.Time{},
	}

	symbol := strings.ToUpper(strings.TrimSpace(form.symbolInput.Value()))
	if symbol == "" {
		return emptyTx, "symbol is required"
	}

	qty, err := strconv.ParseFloat(strings.TrimSpace(form.qtyInput.Value()), 64)
	if err != nil || qty <= 0 {
		return emptyTx, "quantity must be a positive number"
	}

	priceVal, err := strconv.ParseFloat(strings.TrimSpace(form.priceInput.Value()), 64)
	if err != nil || priceVal <= 0 {
		return emptyTx, "price must be a positive number"
	}

	feeStr := strings.TrimSpace(form.feeInput.Value())
	if feeStr == "" {
		feeStr = "0"
	}

	feeVal, err := strconv.ParseFloat(feeStr, 64)
	if err != nil || feeVal < 0 {
		return emptyTx, "fee must be >= 0"
	}

	dateStr := strings.TrimSpace(form.dateInput.Value())

	var executedAt time.Time
	if dateStr == "" {
		executedAt = time.Now().UTC()
	} else {
		parsed, parseErr := time.Parse("2006-01-02", dateStr)
		if parseErr != nil {
			return emptyTx, "date must be YYYY-MM-DD"
		}

		executedAt = parsed.UTC()
	}

	return portfolio.Transaction{
		ID:             form.txID,
		Symbol:         symbol,
		Type:           form.txType,
		Quantity:       qty,
		Price:          priceVal,
		Fee:            feeVal,
		AllocationType: form.allocType,
		Currency:       "",
		ExecutedAt:     executedAt,
		CreatedAt:      time.Time{},
	}, ""
}

func (form transactionFormModel) renderInput(label string, inp textinput.Model, isFocused bool) string {
	lStyle := form.formStyles.label
	if isFocused {
		lStyle = lStyle.Foreground(lipgloss.Color("33"))
	}

	return lStyle.Render(label) + inp.View()
}

func (form transactionFormModel) renderToggle(label, optA, optB string, aSelected, isFocused bool) string {
	styles := form.formStyles

	lStyle := styles.label
	if isFocused {
		lStyle = lStyle.Foreground(lipgloss.Color("33"))
	}

	var rendA, rendB string
	if aSelected {
		rendA = styles.toggleOn.Render(optA)
		rendB = styles.toggleOff.Render(optB)
	} else {
		rendA = styles.toggleOff.Render(optA)
		rendB = styles.toggleOn.Render(optB)
	}

	focusHint := ""
	if isFocused {
		focusHint = styles.hint.Render(" (space: toggle)")
	}

	return lStyle.Render(label) + rendA + " " + rendB + focusHint
}
