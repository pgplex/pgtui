package delegates

import (
	"fmt"
	"log"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rebelice/lazypg/internal/app/messages"
	"github.com/rebelice/lazypg/internal/models"
	"github.com/rebelice/lazypg/internal/ui/components"
)

// ConnectionDelegate handles connection-related messages.
type ConnectionDelegate struct{}

// NewConnectionDelegate creates a new ConnectionDelegate.
func NewConnectionDelegate() *ConnectionDelegate {
	return &ConnectionDelegate{}
}

// Name returns the delegate name.
func (d *ConnectionDelegate) Name() string {
	return "connection"
}

// Update processes connection-related messages.
func (d *ConnectionDelegate) Update(msg tea.Msg, app AppAccess) (bool, tea.Cmd) {
	switch msg := msg.(type) {

	case messages.DiscoveryCompleteMsg:
		// Update connection dialog with discovered instances
		app.GetConnectionDialog().SetDiscoveredInstances(msg.Instances)
		return true, nil

	case messages.ConnectionStartMsg:
		// Start async connection
		app.SetConnecting(true)
		app.SetConnectingStart(time.Now())
		app.SetConnectingConfig(msg.Config)
		// Return both connect command and spinner tick for timer updates
		return true, tea.Batch(app.ConnectAsync(msg.Config), app.GetSpinnerTickCmd())

	case messages.ConnectionResultMsg:
		return d.handleConnectionResult(msg, app)

	case components.PasswordSubmitMsg:
		return d.handlePasswordSubmit(msg, app)

	case components.PasswordCancelMsg:
		// User cancelled password dialog
		app.SetShowPasswordDialog(false)
		app.SetPendingConnectionInfo(nil)
		// Re-show connection dialog
		app.SetShowConnectionDialog(true)
		return true, nil
	}

	return false, nil
}

// handleConnectionResult processes the result of a connection attempt.
func (d *ConnectionDelegate) handleConnectionResult(msg messages.ConnectionResultMsg, app AppAccess) (bool, tea.Cmd) {
	// Ignore result if user already cancelled
	if !app.IsConnecting() {
		return true, nil
	}

	app.SetConnecting(false)

	if msg.Err != nil {
		// Connection failed - clear pending password (don't save wrong password)
		app.ClearPendingPasswordSave()
		app.ShowError("Connection Failed", fmt.Sprintf("Could not connect to %s\n\nError: %v",
			msg.Config.DisplayTarget(), msg.Err))
		return true, nil
	}

	// Connection succeeded - save manually entered password
	if err := app.SavePassword(msg.Config.Host, msg.Config.Port, msg.Config.Database, msg.Config.User, msg.Config.Password); err != nil {
		log.Printf("Warning: Failed to save password: %v", err)
	}

	// Update active connection in state
	connMgr := app.GetConnectionManager()
	if connMgr != nil {
		conn, err := connMgr.GetActive()
		if err == nil && conn != nil {
			app.SetActiveConnection(&models.Connection{
				ID:          msg.ConnID,
				Config:      msg.Config,
				Connected:   conn.Connected,
				ConnectedAt: conn.ConnectedAt,
				LastPing:    conn.LastPing,
				Error:       conn.Error,
			})
		}
	}

	// Save to connection history
	app.AddToConnectionHistory(msg.Config)

	// Hide connection dialog and trigger tree loading
	app.SetShowConnectionDialog(false)

	return true, func() tea.Msg {
		return messages.LoadTreeMsg{}
	}
}

// handlePasswordSubmit processes password submission from dialog.
func (d *ConnectionDelegate) handlePasswordSubmit(msg components.PasswordSubmitMsg, app AppAccess) (bool, tea.Cmd) {
	app.SetShowPasswordDialog(false)

	pendingInfo := app.GetPendingConnectionInfo()
	if pendingInfo == nil {
		return true, nil
	}

	// Create connection config with the entered password
	config := pendingInfo.ToConnectionConfig()
	config.Password = msg.Password

	app.SetPendingConnectionInfo(nil)

	// Start connection
	return true, func() tea.Msg {
		return messages.ConnectionStartMsg{Config: config}
	}
}
