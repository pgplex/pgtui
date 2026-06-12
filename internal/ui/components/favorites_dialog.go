package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/pgplex/pgtui/internal/models"
	"github.com/pgplex/pgtui/internal/ui/theme"
)

// Zone ID prefixes for mouse click handling
const (
	ZoneFavoriteItemPrefix = "fav-item-"
	ZoneFavoriteFieldPrefix = "fav-field-"
)

// FavoritesMode represents the dialog mode
type FavoritesMode int

const (
	FavoritesModeList FavoritesMode = iota
	FavoritesModeAdd
	FavoritesModeEdit
)

// ExecuteFavoriteMsg is sent when a favorite should be executed
type ExecuteFavoriteMsg struct {
	Favorite models.Favorite
}

// CloseFavoritesDialogMsg is sent when dialog should close
type CloseFavoritesDialogMsg struct{}

// AddFavoriteMsg is sent when a new favorite should be added
type AddFavoriteMsg struct {
	Name        string
	Description string
	Query       string
	Tags        []string
}

// EditFavoriteMsg is sent when a favorite should be updated
type EditFavoriteMsg struct {
	FavoriteID  string
	Name        string
	Description string
	Query       string
	Tags        []string
}

// DeleteFavoriteMsg is sent when a favorite should be deleted
type DeleteFavoriteMsg struct {
	FavoriteID string
}

// FavoritesDialog manages favorite queries
type FavoritesDialog struct {
	Width  int
	Height int
	Theme  theme.Theme

	// State
	mode      FavoritesMode
	favorites []models.Favorite
	selected  int
	offset    int

	// Add/Edit state
	nameInput        string
	descriptionInput string
	queryInput       string
	tagsInput        string
	currentField     int // 0=name, 1=description, 2=query, 3=tags

	// Validation and errors
	validationError string

	// Delete confirmation
	deleteConfirmMode bool
}

// NewFavoritesDialog creates a new favorites dialog
func NewFavoritesDialog(th theme.Theme) *FavoritesDialog {
	return &FavoritesDialog{
		Width:     80,
		Height:    30,
		Theme:     th,
		mode:      FavoritesModeList,
		favorites: []models.Favorite{},
		selected:  0,
		offset:    0,
	}
}

// SetFavorites updates the favorites list
func (fd *FavoritesDialog) SetFavorites(favorites []models.Favorite) {
	fd.favorites = favorites
	fd.selected = 0
	fd.offset = 0
}

// Update handles keyboard input
func (fd *FavoritesDialog) Update(msg tea.KeyMsg) (*FavoritesDialog, tea.Cmd) {
	switch fd.mode {
	case FavoritesModeList:
		return fd.handleListMode(msg)
	case FavoritesModeAdd, FavoritesModeEdit:
		return fd.handleEditMode(msg)
	}
	return fd, nil
}

// HandleMouseWheel handles mouse wheel events for scrolling
// Returns true if the event was handled
func (fd *FavoritesDialog) HandleMouseWheel(msg tea.MouseMsg) bool {
	if fd.mode != FavoritesModeList {
		return false
	}

	visibleHeight := fd.Height - 10
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		// Scroll viewport up (like lazygit)
		fd.deleteConfirmMode = false
		fd.offset -= 3
		if fd.offset < 0 {
			fd.offset = 0
		}
		// Keep selection within visible range
		if fd.selected >= fd.offset+visibleHeight {
			fd.selected = fd.offset + visibleHeight - 1
		}
		if fd.selected < fd.offset {
			fd.selected = fd.offset
		}
		return true
	case tea.MouseButtonWheelDown:
		// Scroll viewport down (like lazygit)
		fd.deleteConfirmMode = false
		maxOffset := len(fd.favorites) - visibleHeight
		if maxOffset < 0 {
			maxOffset = 0
		}
		fd.offset += 3
		if fd.offset > maxOffset {
			fd.offset = maxOffset
		}
		// Keep selection within visible range
		if fd.selected < fd.offset {
			fd.selected = fd.offset
		}
		if fd.selected >= fd.offset+visibleHeight {
			fd.selected = fd.offset + visibleHeight - 1
		}
		// Bounds check
		if fd.selected >= len(fd.favorites) {
			fd.selected = len(fd.favorites) - 1
		}
		if fd.selected < 0 {
			fd.selected = 0
		}
		return true
	}
	return false
}

// HandleMouseClick handles mouse click events
// Returns true if click was handled, and a command if needed
func (fd *FavoritesDialog) HandleMouseClick(msg tea.MouseMsg) (handled bool, cmd tea.Cmd) {
	// Only handle left click press events
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return false, nil
	}

	// In list mode, check for item clicks
	if fd.mode == FavoritesModeList {
		visibleStart := fd.offset
		visibleEnd := fd.offset + fd.Height - 10
		if visibleEnd > len(fd.favorites) {
			visibleEnd = len(fd.favorites)
		}

		for i := visibleStart; i < visibleEnd; i++ {
			zoneID := fmt.Sprintf("%s%d", ZoneFavoriteItemPrefix, i)
			if zone.Get(zoneID).InBounds(msg) {
				// Click on already selected executes
				if i == fd.selected {
					fav := fd.favorites[fd.selected]
					return true, func() tea.Msg {
						return ExecuteFavoriteMsg{Favorite: fav}
					}
				}
				// First click selects
				fd.selected = i
				fd.deleteConfirmMode = false
				return true, nil
			}
		}
	}

	// In edit mode, check for field clicks
	if fd.mode == FavoritesModeAdd || fd.mode == FavoritesModeEdit {
		for i := 0; i < 4; i++ {
			zoneID := fmt.Sprintf("%s%d", ZoneFavoriteFieldPrefix, i)
			if zone.Get(zoneID).InBounds(msg) {
				fd.currentField = i
				fd.validationError = ""
				return true, nil
			}
		}
	}

	return false, nil
}

func (fd *FavoritesDialog) handleListMode(msg tea.KeyMsg) (*FavoritesDialog, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		// Cancel delete confirmation if active
		if fd.deleteConfirmMode {
			fd.deleteConfirmMode = false
			return fd, nil
		}
		return fd, func() tea.Msg {
			return CloseFavoritesDialogMsg{}
		}
	case "up", "k":
		// Cancel delete confirmation on navigation
		fd.deleteConfirmMode = false
		if fd.selected > 0 {
			fd.selected--
			if fd.selected < fd.offset {
				fd.offset = fd.selected
			}
		}
	case "down", "j":
		// Cancel delete confirmation on navigation
		fd.deleteConfirmMode = false
		if fd.selected < len(fd.favorites)-1 {
			fd.selected++
			visibleHeight := fd.Height - 10
			if fd.selected >= fd.offset+visibleHeight {
				fd.offset = fd.selected - visibleHeight + 1
			}
		}
	case "enter":
		// Execute selected favorite
		if len(fd.favorites) == 0 {
			return fd, nil
		}
		if fd.selected < len(fd.favorites) {
			fav := fd.favorites[fd.selected]
			return fd, func() tea.Msg {
				return ExecuteFavoriteMsg{Favorite: fav}
			}
		}
	case "a", "n":
		// Add new favorite
		fd.mode = FavoritesModeAdd
		fd.nameInput = ""
		fd.descriptionInput = ""
		fd.queryInput = ""
		fd.tagsInput = ""
		fd.currentField = 0
		fd.validationError = ""
		fd.deleteConfirmMode = false
	case "e":
		// Edit selected favorite
		if len(fd.favorites) == 0 {
			return fd, nil
		}
		if fd.selected < len(fd.favorites) {
			fav := fd.favorites[fd.selected]
			fd.mode = FavoritesModeEdit
			fd.nameInput = fav.Name
			fd.descriptionInput = fav.Description
			fd.queryInput = fav.Query
			fd.tagsInput = strings.Join(fav.Tags, ", ")
			fd.currentField = 0
			fd.validationError = ""
			fd.deleteConfirmMode = false
		}
	case "d", "x":
		// Delete selected favorite with confirmation
		if len(fd.favorites) == 0 {
			return fd, nil
		}
		if fd.selected < len(fd.favorites) {
			if fd.deleteConfirmMode {
				// Second press - confirm delete
				fav := fd.favorites[fd.selected]
				fd.deleteConfirmMode = false
				return fd, func() tea.Msg {
					return DeleteFavoriteMsg{FavoriteID: fav.ID}
				}
			} else {
				// First press - enter confirmation mode
				fd.deleteConfirmMode = true
			}
		}
	}
	return fd, nil
}

func (fd *FavoritesDialog) handleEditMode(msg tea.KeyMsg) (*FavoritesDialog, tea.Cmd) {
	switch msg.String() {
	case "esc":
		fd.mode = FavoritesModeList
		fd.validationError = ""
	case "tab":
		fd.currentField = (fd.currentField + 1) % 4
		fd.validationError = "" // Clear validation error when moving between fields
	case "shift+tab":
		fd.currentField = (fd.currentField - 1 + 4) % 4
		fd.validationError = "" // Clear validation error when moving between fields
	case "backspace":
		fd.deleteChar()
	case "enter":
		if fd.currentField == 3 {
			// Validate inputs before saving
			if err := fd.validateInputs(); err != nil {
				fd.validationError = err.Error()
				return fd, nil
			}

			// Parse tags from comma-separated string
			tags := []string{}
			if fd.tagsInput != "" {
				for _, tag := range strings.Split(fd.tagsInput, ",") {
					tag = strings.TrimSpace(tag)
					if tag != "" {
						tags = append(tags, tag)
					}
				}
			}

			// Send appropriate message based on mode
			switch fd.mode {
			case FavoritesModeAdd:
				cmd := func() tea.Msg {
					return AddFavoriteMsg{
						Name:        strings.TrimSpace(fd.nameInput),
						Description: strings.TrimSpace(fd.descriptionInput),
						Query:       strings.TrimSpace(fd.queryInput),
						Tags:        tags,
					}
				}
				fd.mode = FavoritesModeList
				fd.clearInputs()
				return fd, cmd
			case FavoritesModeEdit:
				fav := fd.favorites[fd.selected]
				cmd := func() tea.Msg {
					return EditFavoriteMsg{
						FavoriteID:  fav.ID,
						Name:        strings.TrimSpace(fd.nameInput),
						Description: strings.TrimSpace(fd.descriptionInput),
						Query:       strings.TrimSpace(fd.queryInput),
						Tags:        tags,
					}
				}
				fd.mode = FavoritesModeList
				fd.clearInputs()
				return fd, cmd
			default:
				fd.mode = FavoritesModeList
			}
		} else {
			fd.currentField++
		}
	default:
		if len(msg.String()) == 1 {
			fd.addChar(msg.String())
		}
	}
	return fd, nil
}

func (fd *FavoritesDialog) addChar(ch string) {
	switch fd.currentField {
	case 0:
		fd.nameInput += ch
	case 1:
		fd.descriptionInput += ch
	case 2:
		fd.queryInput += ch
	case 3:
		fd.tagsInput += ch
	}
}

func (fd *FavoritesDialog) deleteChar() {
	switch fd.currentField {
	case 0:
		if len(fd.nameInput) > 0 {
			fd.nameInput = fd.nameInput[:len(fd.nameInput)-1]
		}
	case 1:
		if len(fd.descriptionInput) > 0 {
			fd.descriptionInput = fd.descriptionInput[:len(fd.descriptionInput)-1]
		}
	case 2:
		if len(fd.queryInput) > 0 {
			fd.queryInput = fd.queryInput[:len(fd.queryInput)-1]
		}
	case 3:
		if len(fd.tagsInput) > 0 {
			fd.tagsInput = fd.tagsInput[:len(fd.tagsInput)-1]
		}
	}
}

func (fd *FavoritesDialog) clearInputs() {
	fd.nameInput = ""
	fd.descriptionInput = ""
	fd.queryInput = ""
	fd.tagsInput = ""
	fd.currentField = 0
	fd.validationError = ""
}

// validateInputs validates the form inputs
func (fd *FavoritesDialog) validateInputs() error {
	name := strings.TrimSpace(fd.nameInput)
	query := strings.TrimSpace(fd.queryInput)

	if name == "" {
		return fmt.Errorf("name is required")
	}

	if query == "" {
		return fmt.Errorf("query is required")
	}

	if len(name) > 100 {
		return fmt.Errorf("name is too long (max 100 characters)")
	}

	return nil
}

// View renders the dialog
func (fd *FavoritesDialog) View() string {
	switch fd.mode {
	case FavoritesModeList:
		return fd.renderList()
	case FavoritesModeAdd, FavoritesModeEdit:
		return fd.renderEdit()
	}
	return ""
}

func (fd *FavoritesDialog) renderList() string {
	var sections []string

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(fd.Theme.Foreground).
		Background(fd.Theme.Info).
		Padding(0, 1).
		Bold(true)
	sections = append(sections, titleStyle.Render("Favorite Queries"))

	// Instructions
	instrStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#a6adc8")).
		Padding(0, 1)
	sections = append(sections, instrStyle.Render("↑↓: Navigate  Enter: Execute  a: Add  e: Edit  d: Delete  Esc: Close"))

	// Delete confirmation warning
	if fd.deleteConfirmMode && len(fd.favorites) > 0 {
		warningStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f38ba8")).
			Background(lipgloss.Color("#45475a")).
			Padding(0, 1).
			Bold(true)
		sections = append(sections, warningStyle.Render("⚠ Press 'd' again to confirm deletion, or Esc to cancel"))
	}

	// Favorites list
	if len(fd.favorites) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#a6adc8")).
			Padding(1, 1)
		emptyMsg := "No favorites yet.\n\nPress 'a' to add your first favorite query.\n\nFavorites let you save frequently used queries for quick access."
		sections = append(sections, emptyStyle.Render(emptyMsg))
	} else {
		sections = append(sections, "")
		visibleStart := fd.offset
		visibleEnd := fd.offset + fd.Height - 10
		if visibleEnd > len(fd.favorites) {
			visibleEnd = len(fd.favorites)
		}

		for i := visibleStart; i < visibleEnd; i++ {
			fav := fd.favorites[i]

			// Format favorite entry
			name := fav.Name
			if len(name) > 40 {
				name = name[:37] + "..."
			}

			desc := fav.Description
			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}

			line := fmt.Sprintf("%s\n  %s", name, desc)
			if len(fav.Tags) > 0 {
				line += fmt.Sprintf(" [%s]", strings.Join(fav.Tags, ", "))
			}

			style := lipgloss.NewStyle().Padding(0, 1)
			if i == fd.selected {
				if fd.deleteConfirmMode {
					// Show red highlight when delete confirmation is active
					style = style.Background(lipgloss.Color("#f38ba8")).Foreground(lipgloss.Color("#1e1e2e"))
				} else {
					style = style.Background(fd.Theme.Selection).Foreground(fd.Theme.Foreground)
				}
			}
			renderedLine := style.Render(line)
			// Wrap with zone mark for mouse click
			zoneID := fmt.Sprintf("%s%d", ZoneFavoriteItemPrefix, i)
			sections = append(sections, zone.Mark(zoneID, renderedLine))
		}
	}

	// Container
	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(fd.Theme.Border).
		Width(fd.Width).
		Height(fd.Height).
		Padding(1)

	return containerStyle.Render(strings.Join(sections, "\n"))
}

func (fd *FavoritesDialog) renderEdit() string {
	var sections []string

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(fd.Theme.Foreground).
		Background(fd.Theme.Info).
		Padding(0, 1).
		Bold(true)

	title := "Add Favorite"
	if fd.mode == FavoritesModeEdit {
		title = "Edit Favorite"
	}
	sections = append(sections, titleStyle.Render(title))

	// Instructions
	instrStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#a6adc8")).
		Padding(0, 1)
	sections = append(sections, instrStyle.Render("Tab/Shift+Tab: Navigate fields  Enter: Save  Esc: Cancel"))

	// Validation error
	if fd.validationError != "" {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f38ba8")).
			Background(lipgloss.Color("#45475a")).
			Padding(0, 1).
			Bold(true)
		sections = append(sections, errorStyle.Render("⚠ "+fd.validationError))
	}

	// Fields with zone marks for mouse clicks
	sections = append(sections, "")
	sections = append(sections, zone.Mark(ZoneFavoriteFieldPrefix+"0", fd.renderField("Name: (required)", fd.nameInput, fd.currentField == 0)))
	sections = append(sections, zone.Mark(ZoneFavoriteFieldPrefix+"1", fd.renderField("Description:", fd.descriptionInput, fd.currentField == 1)))
	sections = append(sections, zone.Mark(ZoneFavoriteFieldPrefix+"2", fd.renderField("Query: (required)", fd.queryInput, fd.currentField == 2)))
	sections = append(sections, zone.Mark(ZoneFavoriteFieldPrefix+"3", fd.renderField("Tags: (comma separated, optional)", fd.tagsInput, fd.currentField == 3)))

	// Help text
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Padding(1, 1)
	helpMsg := "Press Tab to move between fields.\nPress Enter on the last field to save."
	sections = append(sections, helpStyle.Render(helpMsg))

	// Container
	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(fd.Theme.Border).
		Width(fd.Width).
		Height(fd.Height).
		Padding(1)

	return containerStyle.Render(strings.Join(sections, "\n"))
}

func (fd *FavoritesDialog) renderField(label, value string, active bool) string {
	style := lipgloss.NewStyle().Padding(0, 1)
	if active {
		style = style.Background(fd.Theme.Selection).Foreground(fd.Theme.Foreground)
		value = value + "_"
	}
	return style.Render(fmt.Sprintf("%s %s", label, value))
}

// GetEditData returns the current edit data
func (fd *FavoritesDialog) GetEditData() (name, description, query string, tags []string) {
	name = fd.nameInput
	description = fd.descriptionInput
	query = fd.queryInput

	// Parse tags
	if fd.tagsInput != "" {
		parts := strings.Split(fd.tagsInput, ",")
		for _, part := range parts {
			tag := strings.TrimSpace(part)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}

	return
}
