package connection_history

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const passwordSalt = "pgtui-keyring-salt-v1"

// deriveFilePassword generates a machine-specific password for the file backend.
// This password is derived from machine ID and username, so it's consistent
// across app restarts but different on each machine.
func deriveFilePassword() (string, error) {
	machineID, err := getMachineID()
	if err != nil {
		// Fallback to hostname if machine ID is unavailable
		machineID, _ = os.Hostname()
	}

	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("USERNAME") // Windows
	}
	if username == "" {
		// Fallback for containers/service accounts without USER env
		username = fmt.Sprintf("uid-%d", os.Getuid())
	}

	// Combine machine ID, username, and salt to create a unique password
	data := machineID + username + passwordSalt
	hash := sha256.Sum256([]byte(data))

	return base64.StdEncoding.EncodeToString(hash[:]), nil
}

// getMachineID returns a unique identifier for the current machine.
func getMachineID() (string, error) {
	switch runtime.GOOS {
	case "linux":
		return getLinuxMachineID()
	case "darwin":
		return getDarwinMachineID()
	case "windows":
		return getWindowsMachineID()
	default:
		hostname, err := os.Hostname()
		return hostname, err
	}
}

// getLinuxMachineID reads the machine ID from /etc/machine-id or /var/lib/dbus/machine-id
func getLinuxMachineID() (string, error) {
	paths := []string{
		"/etc/machine-id",
		"/var/lib/dbus/machine-id",
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(data)), nil
		}
	}

	// Fallback to hostname
	return os.Hostname()
}

// getDarwinMachineID gets the hardware UUID on macOS
func getDarwinMachineID() (string, error) {
	cmd := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
	output, err := cmd.Output()
	if err != nil {
		return os.Hostname()
	}

	// Parse IOPlatformUUID from output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "IOPlatformUUID") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				uuid := strings.TrimSpace(parts[1])
				uuid = strings.Trim(uuid, "\"")
				return uuid, nil
			}
		}
	}

	return os.Hostname()
}

// getWindowsMachineID gets the machine GUID on Windows
func getWindowsMachineID() (string, error) {
	cmd := exec.Command("wmic", "csproduct", "get", "UUID")
	output, err := cmd.Output()
	if err != nil {
		return os.Hostname()
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && line != "UUID" {
			return line, nil
		}
	}

	return os.Hostname()
}
