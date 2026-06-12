package connection_history

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/99designs/keyring"
)

const serviceName = "pgtui"

// PasswordStore handles secure password storage using OS keyring with file fallback
type PasswordStore struct {
	ring          keyring.Keyring
	usingFallback bool
}

// NewPasswordStore creates a new password store with platform-appropriate backends
func NewPasswordStore(configDir string) (*PasswordStore, error) {
	backends := getBackendsForPlatform()
	fileDir := filepath.Join(configDir, "keyring")

	ring, err := keyring.Open(keyring.Config{
		ServiceName:     serviceName,
		AllowedBackends: backends,
		// macOS Keychain configuration:
		// - Use "login" keychain which is automatically unlocked when user logs in
		//   This avoids the need for separate keychain password prompts
		// - Trust the application to reduce "allow access" prompts
		KeychainName:             "login",
		KeychainTrustApplication: true,
		// File backend configuration (fallback)
		FileDir: fileDir,
		FilePasswordFunc: func(_ string) (string, error) {
			return deriveFilePassword()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open keyring: %w", err)
	}

	// Check if we're using the file backend (fallback)
	usingFallback := isUsingFallback(backends, ring)

	return &PasswordStore{
		ring:          ring,
		usingFallback: usingFallback,
	}, nil
}

// getBackendsForPlatform returns the appropriate backend priority for the current OS
func getBackendsForPlatform() []keyring.BackendType {
	switch runtime.GOOS {
	case "darwin":
		return []keyring.BackendType{
			keyring.KeychainBackend,
			keyring.FileBackend,
		}
	case "linux":
		return []keyring.BackendType{
			keyring.SecretServiceBackend,
			keyring.KWalletBackend,
			keyring.FileBackend,
		}
	case "windows":
		return []keyring.BackendType{
			keyring.WinCredBackend,
			keyring.FileBackend,
		}
	default:
		return []keyring.BackendType{
			keyring.FileBackend,
		}
	}
}

// isUsingFallback checks if the opened keyring is using the file backend
func isUsingFallback(requestedBackends []keyring.BackendType, ring keyring.Keyring) bool {
	// If file backend is the only option, we're using fallback
	if len(requestedBackends) == 1 && requestedBackends[0] == keyring.FileBackend {
		return true
	}

	// Try to detect by checking available backends
	availableBackends := keyring.AvailableBackends()
	for _, b := range availableBackends {
		if b != keyring.FileBackend {
			// A native backend is available, likely not using fallback
			return false
		}
	}

	return true
}

// IsUsingFallback returns true if the password store is using the file backend
// instead of the native OS keyring
func (ps *PasswordStore) IsUsingFallback() bool {
	return ps.usingFallback
}

// Save stores a password securely in the keyring
// key format: "host:port:database:user" for uniqueness
func (ps *PasswordStore) Save(host string, port int, database, user, password string) error {
	if password == "" {
		// Don't save empty passwords
		return nil
	}

	key := makeKey(host, port, database, user)
	err := ps.ring.Set(keyring.Item{
		Key:         key,
		Data:        []byte(password),
		Label:       fmt.Sprintf("pgtui: %s@%s:%d/%s", user, host, port, database),
		Description: "PostgreSQL connection password for pgtui",
	})
	if err != nil {
		return &PasswordSaveError{
			Err:     err,
			Message: "failed to save password to keyring",
		}
	}
	return nil
}

// Get retrieves a password from the keyring
func (ps *PasswordStore) Get(host string, port int, database, user string) (string, error) {
	key := makeKey(host, port, database, user)
	item, err := ps.ring.Get(key)
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return "", ErrPasswordNotFound
		}
		return "", &PasswordReadError{Err: err}
	}
	return string(item.Data), nil
}

// Delete removes a password from the keyring
func (ps *PasswordStore) Delete(host string, port int, database, user string) error {
	key := makeKey(host, port, database, user)
	err := ps.ring.Remove(key)
	if err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
		return fmt.Errorf("failed to delete password from keyring: %w", err)
	}
	return nil
}

// makeKey creates a unique key for password storage
func makeKey(host string, port int, database, user string) string {
	return fmt.Sprintf("%s:%d:%s:%s", host, port, database, user)
}
