package components

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/pgplex/pgtui/internal/ui/theme"
)

// CodeEditorCloseMsg is sent when the code editor should be closed
type CodeEditorCloseMsg struct{}

// CopySuccessMsg is sent when content is copied to clipboard
type CopySuccessMsg struct {
	BytesCopied int
}

// SaveObjectMsg is sent when user wants to save changes
type SaveObjectMsg struct {
	ObjectType string // "function", "procedure", "view", etc.
	ObjectName string // "public.get_user_by_id"
	Content    string // New definition
}

// ObjectSavedMsg is sent after save completes
type ObjectSavedMsg struct {
	Success bool
	Error   error
}

// CodeEditor is a component for viewing and editing database object definitions
type CodeEditor struct {
	// Content
	lines     []string
	cursorRow int
	cursorCol int
	scrollY   int

	// Object info
	Title      string // e.g., "Function: public.get_user_by_id(integer)"
	ObjectType string // "function", "procedure", "view", etc.
	ObjectName string // "public.get_user_by_id"
	Language   string // "plpgsql", "sql"

	// State
	Width         int
	Height        int
	ReadOnly      bool   // true = view mode, false = edit mode
	Modified      bool   // true if content changed from original
	Original      string // Original content for comparison
	Focused       bool   // true if this editor has focus
	statusMessage string // Temporary status message (e.g., "✓ Copied")

	// Theme
	Theme theme.Theme

	// Cached styles
	cachedStyles *codeEditorStyles

	// Chroma formatter (cached for performance)
	chromaStyle     *chroma.Style
	chromaFormatter chroma.Formatter
}

// codeEditorStyles holds pre-computed styles
type codeEditorStyles struct {
	border         lipgloss.Style
	borderFocused  lipgloss.Style
	lineNumber     lipgloss.Style
	lineNumberSep  lipgloss.Style
	content        lipgloss.Style
	title          lipgloss.Style
	modeReadOnly   lipgloss.Style
	modeEditing    lipgloss.Style
	modeModified   lipgloss.Style
	statusBar      lipgloss.Style
	cursor         lipgloss.Style
	emptyLine      lipgloss.Style
}

// NewCodeEditor creates a new code editor
func NewCodeEditor(th theme.Theme) *CodeEditor {
	ce := &CodeEditor{
		lines:    []string{""},
		ReadOnly: true,
		Theme:    th,
		Language: "sql",
	}
	ce.initStyles()
	ce.initChroma()
	return ce
}

// initStyles initializes cached styles
func (ce *CodeEditor) initStyles() {
	ce.cachedStyles = &codeEditorStyles{
		border: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ce.Theme.Border),
		borderFocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ce.Theme.BorderFocused),
		lineNumber: lipgloss.NewStyle().
			Foreground(ce.Theme.Metadata),
		lineNumberSep: lipgloss.NewStyle().
			Foreground(ce.Theme.Border),
		content: lipgloss.NewStyle().
			Foreground(ce.Theme.Foreground),
		title: lipgloss.NewStyle().
			Foreground(ce.Theme.Info).
			Bold(true),
		modeReadOnly: lipgloss.NewStyle().
			Foreground(ce.Theme.Metadata),
		modeEditing: lipgloss.NewStyle().
			Foreground(ce.Theme.Warning).
			Bold(true),
		modeModified: lipgloss.NewStyle().
			Foreground(ce.Theme.Error).
			Bold(true),
		statusBar: lipgloss.NewStyle().
			Foreground(ce.Theme.Metadata).
			Italic(true),
		cursor: lipgloss.NewStyle().
			Foreground(ce.Theme.Background).
			Background(ce.Theme.Cursor),
		emptyLine: lipgloss.NewStyle().
			Foreground(ce.Theme.Metadata),
	}
}

// initChroma initializes Chroma syntax highlighter
func (ce *CodeEditor) initChroma() {
	// Use a built-in style that works well in terminals
	ce.chromaStyle = styles.Get("monokai")
	if ce.chromaStyle == nil {
		ce.chromaStyle = styles.Fallback
	}

	// Use terminal256 formatter for ANSI output
	ce.chromaFormatter = formatters.Get("terminal256")
	if ce.chromaFormatter == nil {
		ce.chromaFormatter = formatters.Fallback
	}
}

// SetContent sets the editor content
func (ce *CodeEditor) SetContent(content, objectType, title string) {
	if content == "" {
		ce.lines = []string{""}
	} else {
		ce.lines = strings.Split(content, "\n")
	}
	ce.Original = content
	ce.ObjectType = objectType
	ce.Title = title
	ce.Modified = false
	ce.cursorRow = 0
	ce.cursorCol = 0
	ce.scrollY = 0

	// Set language based on object type
	switch objectType {
	case "function", "procedure", "trigger_function":
		ce.Language = "plpgsql"
	default:
		ce.Language = "sql"
	}
}

// GetContent returns the full content as a single string
func (ce *CodeEditor) GetContent() string {
	return strings.Join(ce.lines, "\n")
}

// EnterEditMode switches to edit mode
func (ce *CodeEditor) EnterEditMode() {
	ce.ReadOnly = false
}

// ExitEditMode switches to read-only mode, optionally discarding changes
func (ce *CodeEditor) ExitEditMode(discardChanges bool) {
	if discardChanges && ce.Modified {
		// Restore original content
		ce.SetContent(ce.Original, ce.ObjectType, ce.Title)
	}
	ce.ReadOnly = true
}

// CopyContent copies the content to clipboard
func (ce *CodeEditor) CopyContent() error {
	return clipboard.WriteAll(ce.GetContent())
}

// highlightLine applies syntax highlighting to a single line
func (ce *CodeEditor) highlightLine(line string) string {
	if line == "" {
		return ""
	}

	// Get appropriate lexer
	var lexer chroma.Lexer
	switch ce.Language {
	case "plpgsql":
		lexer = lexers.Get("plpgsql")
		if lexer == nil {
			lexer = lexers.Get("postgresql")
		}
	default:
		lexer = lexers.Get("postgresql")
	}
	if lexer == nil {
		lexer = lexers.Get("sql")
	}
	if lexer == nil {
		// Fallback: return plain text
		return ce.cachedStyles.content.Render(line)
	}

	// Coalesce runs of tokens to reduce output
	lexer = chroma.Coalesce(lexer)

	// Tokenize the line
	iterator, err := lexer.Tokenise(nil, line)
	if err != nil {
		return ce.cachedStyles.content.Render(line)
	}

	// Format to terminal256
	var buf bytes.Buffer
	err = ce.chromaFormatter.Format(&buf, ce.chromaStyle, iterator)
	if err != nil {
		return ce.cachedStyles.content.Render(line)
	}

	// Remove trailing newline added by chroma
	result := strings.TrimSuffix(buf.String(), "\n")
	return result
}

// getLineNumberWidth returns the width needed for line numbers
func (ce *CodeEditor) getLineNumberWidth() int {
	maxLine := len(ce.lines)
	if maxLine < 10 {
		maxLine = 10
	}
	digits := len(fmt.Sprintf("%d", maxLine))
	if digits < 2 {
		digits = 2
	}
	return digits + 3 // digits + space + separator + space
}

// renderLine renders a single line with line number
func (ce *CodeEditor) renderLine(lineNum int, hasCursor bool, contentWidth int) string {
	lineNumWidth := ce.getLineNumberWidth()
	lineNumStr := fmt.Sprintf("%*d", lineNumWidth-3, lineNum+1)

	lineNumPart := ce.cachedStyles.lineNumber.Render(lineNumStr) +
		ce.cachedStyles.lineNumberSep.Render(" │ ")

	// Get line content
	line := ""
	if lineNum < len(ce.lines) {
		line = ce.lines[lineNum]
	}

	// Calculate available width for content
	availableWidth := contentWidth - lineNumWidth
	if availableWidth < 10 {
		availableWidth = 10
	}

	// Truncate line if too long
	displayLine := line
	if runewidth.StringWidth(displayLine) > availableWidth {
		displayLine = runewidth.Truncate(displayLine, availableWidth-1, "…")
	}

	// Apply syntax highlighting
	var contentPart string
	if hasCursor && !ce.ReadOnly {
		// In edit mode with cursor, render with cursor
		contentPart = ce.renderLineWithCursor(displayLine, availableWidth)
	} else {
		contentPart = ce.highlightLine(displayLine)
	}

	return lineNumPart + contentPart
}

// renderLineWithCursor renders a line with cursor for edit mode
func (ce *CodeEditor) renderLineWithCursor(line string, maxWidth int) string {
	// Simple cursor rendering without syntax highlighting for now
	// (combining cursor and syntax highlighting is complex)
	runes := []rune(line)

	var result strings.Builder
	for i, r := range runes {
		if i == ce.cursorCol {
			result.WriteString(ce.cachedStyles.cursor.Render(string(r)))
		} else {
			result.WriteString(ce.cachedStyles.content.Render(string(r)))
		}
	}

	// Cursor at end of line
	if ce.cursorCol >= len(runes) {
		result.WriteString(ce.cachedStyles.cursor.Render(" "))
	}

	return result.String()
}

// renderEmptyLine renders an empty line placeholder
func (ce *CodeEditor) renderEmptyLine(lineNum int) string {
	lineNumWidth := ce.getLineNumberWidth()
	lineNumStr := fmt.Sprintf("%*s", lineNumWidth-3, "~")

	return ce.cachedStyles.emptyLine.Render(lineNumStr) +
		ce.cachedStyles.lineNumberSep.Render(" │ ")
}

// View renders the code editor
func (ce *CodeEditor) View() string {
	if ce.Width <= 0 || ce.Height <= 0 {
		return ""
	}

	// Choose border style based on focus and edit mode
	borderStyle := ce.cachedStyles.border
	if ce.Focused || !ce.ReadOnly {
		borderStyle = ce.cachedStyles.borderFocused
	}

	// Calculate dimensions
	frameSize := borderStyle.GetHorizontalFrameSize()
	contentWidth := ce.Width - frameSize
	if contentWidth < 20 {
		contentWidth = 20
	}

	verticalFrameSize := borderStyle.GetVerticalFrameSize()
	// Reserve: 1 for title, 1 for title separator, 1 for bottom separator, 1 for status bar
	contentHeight := ce.Height - verticalFrameSize - 4
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Build title bar with separator below
	titleBar := ce.renderTitleBar(contentWidth)
	titleSeparator := ce.renderSeparator(contentWidth)

	// Build content lines
	var contentLines []string

	// Ensure cursor is visible (scroll if needed)
	ce.ensureCursorVisible(contentHeight)

	// Render visible lines
	startLine := ce.scrollY
	endLine := startLine + contentHeight
	if endLine > len(ce.lines) {
		endLine = len(ce.lines)
	}

	for i := startLine; i < endLine; i++ {
		hasCursor := (i == ce.cursorRow) && !ce.ReadOnly
		contentLines = append(contentLines, ce.renderLine(i, hasCursor, contentWidth))
	}

	// Pad with empty lines if needed
	for len(contentLines) < contentHeight {
		contentLines = append(contentLines, ce.renderEmptyLine(startLine+len(contentLines)))
	}

	// Build separator
	separator := ce.renderSeparator(contentWidth)

	// Build status bar
	statusBar := ce.renderStatusBar(contentWidth)

	// Join all parts
	allLines := []string{titleBar, titleSeparator}
	allLines = append(allLines, contentLines...)
	allLines = append(allLines, separator, statusBar)

	content := strings.Join(allLines, "\n")

	// Apply border
	return borderStyle.Width(contentWidth).Render(content)
}

// getObjectIcon returns the icon and color for the object type
func (ce *CodeEditor) getObjectIcon() (string, lipgloss.Color) {
	switch ce.ObjectType {
	case "function":
		return "ƒ", ce.Theme.FunctionIcon
	case "procedure":
		return "⚙", ce.Theme.ProcedureIcon
	case "trigger_function":
		return "⚡", ce.Theme.TriggerFunctionIcon
	case "view":
		return "◎", ce.Theme.ViewIcon
	case "materialized_view":
		return "◉", ce.Theme.MaterializedViewIcon
	case "sequence":
		return "#", ce.Theme.SequenceIcon
	case "index":
		return "⊕", ce.Theme.IndexIcon
	case "trigger":
		return "↯", ce.Theme.TriggerIcon
	case "extension":
		return "◈", ce.Theme.ExtensionIcon
	case "type", "composite_type":
		return "◫", ce.Theme.TypeIcon
	case "enum_type":
		return "◧", ce.Theme.TypeIcon
	case "domain_type":
		return "◨", ce.Theme.TypeIcon
	case "range_type":
		return "◩", ce.Theme.TypeIcon
	default:
		return "□", ce.Theme.Foreground
	}
}

// renderTitleBar renders the title bar with mode indicator
func (ce *CodeEditor) renderTitleBar(width int) string {
	// Get icon for object type
	icon, iconColor := ce.getObjectIcon()
	iconStyled := lipgloss.NewStyle().Foreground(iconColor).Render(icon)

	// Title on left (with icon)
	title := ce.Title
	if title == "" {
		title = "Code Editor"
	}

	// Mode indicator on right
	var modeIndicator string
	if ce.ReadOnly {
		modeIndicator = ce.cachedStyles.modeReadOnly.Render("[Read Only]")
	} else if ce.Modified {
		modeIndicator = ce.cachedStyles.modeModified.Render("[Modified *]")
	} else {
		modeIndicator = ce.cachedStyles.modeEditing.Render("[Editing]")
	}

	modeWidth := runewidth.StringWidth("[Modified *]") // Use max width for consistent layout
	iconWidth := runewidth.StringWidth(icon) + 1       // +1 for space after icon
	titleMaxWidth := width - modeWidth - iconWidth - 2 // 2 for spacing

	if runewidth.StringWidth(title) > titleMaxWidth {
		title = runewidth.Truncate(title, titleMaxWidth-3, "...")
	}

	titleRendered := iconStyled + " " + ce.cachedStyles.title.Render(title)
	titleWidth := iconWidth + runewidth.StringWidth(title)

	// Calculate padding between title and mode
	padding := width - titleWidth - runewidth.StringWidth(modeIndicator)
	if padding < 1 {
		padding = 1
	}

	return titleRendered + strings.Repeat(" ", padding) + modeIndicator
}

// renderSeparator renders the separator line
func (ce *CodeEditor) renderSeparator(width int) string {
	return ce.cachedStyles.lineNumberSep.Render(strings.Repeat("─", width))
}

// renderStatusBar renders the status bar with help and position info
func (ce *CodeEditor) renderStatusBar(width int) string {
	var helpParts []string

	if ce.ReadOnly {
		helpParts = []string{"e:edit", "y:copy", "esc:close"}

		// Show scroll hint if content is scrollable
		if len(ce.lines) > ce.Height-5 {
			helpParts = append([]string{"j/k:scroll"}, helpParts...)
		}
	} else {
		helpParts = []string{"Ctrl+S:save", "Esc:cancel"}
	}

	helpText := strings.Join(helpParts, "  ")

	// Position info on right
	totalLines := len(ce.lines)
	lineNum := ce.cursorRow + 1
	colNum := ce.cursorCol + 1

	var posInfo string
	if ce.ReadOnly {
		// Show line/total and percentage
		percent := 100
		if totalLines > 1 {
			percent = (ce.scrollY * 100) / (totalLines - 1)
			if percent > 100 {
				percent = 100
			}
		}
		posInfo = fmt.Sprintf("Line %d/%d  %d%%", lineNum, totalLines, percent)
	} else {
		posInfo = fmt.Sprintf("Ln %d, Col %d", lineNum, colNum)
	}

	// Add status message if present
	statusText := ""
	if ce.statusMessage != "" {
		// Use green for success (✓), yellow for warnings (⚠)
		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")) // Green
		if strings.HasPrefix(ce.statusMessage, "⚠") {
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af")) // Yellow
		}
		statusText = "  " + statusStyle.Render(ce.statusMessage)
	}

	// Calculate spacing
	helpWidth := runewidth.StringWidth(helpText)
	statusWidth := runewidth.StringWidth(ce.statusMessage)
	if statusWidth > 0 {
		statusWidth += 2 // for "  " prefix
	}
	posWidth := runewidth.StringWidth(posInfo)
	padding := width - helpWidth - statusWidth - posWidth
	if padding < 1 {
		padding = 1
	}

	return ce.cachedStyles.statusBar.Render(helpText) +
		statusText +
		strings.Repeat(" ", padding) +
		ce.cachedStyles.statusBar.Render(posInfo)
}

// ensureCursorVisible adjusts scroll to keep cursor in view
func (ce *CodeEditor) ensureCursorVisible(viewportHeight int) {
	// Scroll up if cursor is above viewport
	if ce.cursorRow < ce.scrollY {
		ce.scrollY = ce.cursorRow
	}

	// Scroll down if cursor is below viewport
	if ce.cursorRow >= ce.scrollY+viewportHeight {
		ce.scrollY = ce.cursorRow - viewportHeight + 1
	}

	// Clamp scroll position
	maxScroll := len(ce.lines) - viewportHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if ce.scrollY > maxScroll {
		ce.scrollY = maxScroll
	}
	if ce.scrollY < 0 {
		ce.scrollY = 0
	}
}

// Update handles keyboard input
func (ce *CodeEditor) Update(msg tea.KeyMsg) (*CodeEditor, tea.Cmd) {
	if ce.ReadOnly {
		return ce.handleReadOnlyKeys(msg)
	}
	return ce.handleEditKeys(msg)
}

// handleReadOnlyKeys handles key events in read-only mode
func (ce *CodeEditor) handleReadOnlyKeys(msg tea.KeyMsg) (*CodeEditor, tea.Cmd) {
	// Clear status message on any key press (except when copying)
	if msg.String() != "y" {
		ce.statusMessage = ""
	}

	switch msg.String() {
	// Scrolling
	case "j", "down":
		ce.scrollDown()
	case "k", "up":
		ce.scrollUp()
	case "g":
		ce.scrollToTop()
	case "G":
		ce.scrollToBottom()
	case "ctrl+d":
		ce.scrollPageDown()
	case "ctrl+u":
		ce.scrollPageUp()

	// Copy
	case "y":
		content := ce.GetContent()
		if err := clipboard.WriteAll(content); err == nil {
			ce.statusMessage = fmt.Sprintf("✓ Copied (%d bytes)", len(content))
		} else {
			ce.statusMessage = fmt.Sprintf("⚠ Copy failed: %v", err)
		}

	// Enter edit mode
	case "e":
		ce.EnterEditMode()

	// Close (only esc, q is reserved for quitting app)
	case "esc":
		return ce, func() tea.Msg {
			return CodeEditorCloseMsg{}
		}
	}

	return ce, nil
}

// handleEditKeys handles key events in edit mode
func (ce *CodeEditor) handleEditKeys(msg tea.KeyMsg) (*CodeEditor, tea.Cmd) {
	switch msg.String() {
	// Cursor movement
	case "left":
		ce.moveCursorLeft()
	case "right":
		ce.moveCursorRight()
	case "up":
		ce.moveCursorUp()
	case "down":
		ce.moveCursorDown()
	case "home":
		ce.moveCursorToLineStart()
	case "end":
		ce.moveCursorToLineEnd()
	case "ctrl+home":
		ce.moveCursorToDocStart()
	case "ctrl+end":
		ce.moveCursorToDocEnd()

	// Text editing
	case "backspace":
		ce.deleteCharBefore()
	case "delete":
		ce.deleteCharAfter()
	case "enter":
		ce.insertNewline()
	case "tab":
		// Insert 4 spaces for tab
		for i := 0; i < 4; i++ {
			ce.insertChar(' ')
		}

	// Save
	case "ctrl+s":
		content := ce.GetContent()
		return ce, func() tea.Msg {
			return SaveObjectMsg{
				ObjectType: ce.ObjectType,
				ObjectName: ce.ObjectName,
				Content:    content,
			}
		}

	// Cancel edit
	case "esc":
		ce.ExitEditMode(true) // Discard changes

	default:
		// Handle printable characters
		if len(msg.String()) == 1 {
			ch := rune(msg.String()[0])
			if ch >= 32 && ch <= 126 {
				ce.insertChar(ch)
			}
		} else if msg.Type == tea.KeyRunes {
			for _, r := range msg.Runes {
				ce.insertChar(r)
			}
		}
	}

	return ce, nil
}

// Scroll methods
func (ce *CodeEditor) scrollDown() {
	if ce.scrollY < len(ce.lines)-1 {
		ce.scrollY++
		ce.cursorRow = ce.scrollY
	}
}

func (ce *CodeEditor) scrollUp() {
	if ce.scrollY > 0 {
		ce.scrollY--
		ce.cursorRow = ce.scrollY
	}
}

func (ce *CodeEditor) scrollToTop() {
	ce.scrollY = 0
	ce.cursorRow = 0
}

func (ce *CodeEditor) scrollToBottom() {
	ce.scrollY = len(ce.lines) - 1
	if ce.scrollY < 0 {
		ce.scrollY = 0
	}
	ce.cursorRow = len(ce.lines) - 1
}

func (ce *CodeEditor) scrollPageDown() {
	pageSize := ce.Height - 5
	if pageSize < 1 {
		pageSize = 1
	}
	ce.scrollY += pageSize
	if ce.scrollY > len(ce.lines)-1 {
		ce.scrollY = len(ce.lines) - 1
	}
	ce.cursorRow = ce.scrollY
}

func (ce *CodeEditor) scrollPageUp() {
	pageSize := ce.Height - 5
	if pageSize < 1 {
		pageSize = 1
	}
	ce.scrollY -= pageSize
	if ce.scrollY < 0 {
		ce.scrollY = 0
	}
	ce.cursorRow = ce.scrollY
}

// Cursor movement methods
func (ce *CodeEditor) moveCursorLeft() {
	if ce.cursorCol > 0 {
		ce.cursorCol--
	} else if ce.cursorRow > 0 {
		ce.cursorRow--
		ce.cursorCol = len(ce.lines[ce.cursorRow])
	}
}

func (ce *CodeEditor) moveCursorRight() {
	lineLen := len(ce.lines[ce.cursorRow])
	if ce.cursorCol < lineLen {
		ce.cursorCol++
	} else if ce.cursorRow < len(ce.lines)-1 {
		ce.cursorRow++
		ce.cursorCol = 0
	}
}

func (ce *CodeEditor) moveCursorUp() {
	if ce.cursorRow > 0 {
		ce.cursorRow--
		lineLen := len(ce.lines[ce.cursorRow])
		if ce.cursorCol > lineLen {
			ce.cursorCol = lineLen
		}
	}
}

func (ce *CodeEditor) moveCursorDown() {
	if ce.cursorRow < len(ce.lines)-1 {
		ce.cursorRow++
		lineLen := len(ce.lines[ce.cursorRow])
		if ce.cursorCol > lineLen {
			ce.cursorCol = lineLen
		}
	}
}

func (ce *CodeEditor) moveCursorToLineStart() {
	ce.cursorCol = 0
}

func (ce *CodeEditor) moveCursorToLineEnd() {
	ce.cursorCol = len(ce.lines[ce.cursorRow])
}

func (ce *CodeEditor) moveCursorToDocStart() {
	ce.cursorRow = 0
	ce.cursorCol = 0
}

func (ce *CodeEditor) moveCursorToDocEnd() {
	ce.cursorRow = len(ce.lines) - 1
	ce.cursorCol = len(ce.lines[ce.cursorRow])
}

// Text editing methods
func (ce *CodeEditor) insertChar(ch rune) {
	line := ce.lines[ce.cursorRow]
	runes := []rune(line)

	// Insert character at cursor position
	newRunes := make([]rune, 0, len(runes)+1)
	newRunes = append(newRunes, runes[:ce.cursorCol]...)
	newRunes = append(newRunes, ch)
	newRunes = append(newRunes, runes[ce.cursorCol:]...)

	ce.lines[ce.cursorRow] = string(newRunes)
	ce.cursorCol++
	ce.Modified = true
}

func (ce *CodeEditor) insertNewline() {
	line := ce.lines[ce.cursorRow]
	runes := []rune(line)

	before := string(runes[:ce.cursorCol])
	after := string(runes[ce.cursorCol:])

	ce.lines[ce.cursorRow] = before

	// Insert new line after current
	newLines := make([]string, len(ce.lines)+1)
	copy(newLines[:ce.cursorRow+1], ce.lines[:ce.cursorRow+1])
	newLines[ce.cursorRow+1] = after
	copy(newLines[ce.cursorRow+2:], ce.lines[ce.cursorRow+1:])
	ce.lines = newLines

	ce.cursorRow++
	ce.cursorCol = 0
	ce.Modified = true
}

func (ce *CodeEditor) deleteCharBefore() {
	if ce.cursorCol > 0 {
		line := ce.lines[ce.cursorRow]
		runes := []rune(line)
		newRunes := append(runes[:ce.cursorCol-1], runes[ce.cursorCol:]...)
		ce.lines[ce.cursorRow] = string(newRunes)
		ce.cursorCol--
		ce.Modified = true
	} else if ce.cursorRow > 0 {
		// Merge with previous line
		prevLine := ce.lines[ce.cursorRow-1]
		currLine := ce.lines[ce.cursorRow]
		ce.cursorCol = len([]rune(prevLine))
		ce.lines[ce.cursorRow-1] = prevLine + currLine

		// Remove current line
		ce.lines = append(ce.lines[:ce.cursorRow], ce.lines[ce.cursorRow+1:]...)
		ce.cursorRow--
		ce.Modified = true
	}
}

func (ce *CodeEditor) deleteCharAfter() {
	line := ce.lines[ce.cursorRow]
	runes := []rune(line)

	if ce.cursorCol < len(runes) {
		newRunes := append(runes[:ce.cursorCol], runes[ce.cursorCol+1:]...)
		ce.lines[ce.cursorRow] = string(newRunes)
		ce.Modified = true
	} else if ce.cursorRow < len(ce.lines)-1 {
		// Merge with next line
		nextLine := ce.lines[ce.cursorRow+1]
		ce.lines[ce.cursorRow] = line + nextLine

		// Remove next line
		ce.lines = append(ce.lines[:ce.cursorRow+1], ce.lines[ce.cursorRow+2:]...)
		ce.Modified = true
	}
}
