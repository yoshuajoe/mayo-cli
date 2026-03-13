package session

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mayo-cli/internal/config"
	"mayo-cli/internal/privacy"
	"github.com/google/uuid"
	"sort"
)

type Session struct {
	ID        string    `json:"id"`
	Summary   string    `json:"summary"`
	VaultKey  string    `json:"vault_key"` // Base64-encoded AES-256 key
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewSession() (*Session, error) {
	id := uuid.New().String()
	sessionDir := filepath.Join(config.GetConfigDir(), "sessions", id)
	
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return nil, err
	}

	// Generate a random 32-byte AES-256 key for this session
	keyBytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, keyBytes); err != nil {
		return nil, fmt.Errorf("failed to generate vault key: %v", err)
	}
	vaultKeyB64 := base64.StdEncoding.EncodeToString(keyBytes)

	session := &Session{
		ID:        id,
		Summary:   "New Research Session",
		VaultKey:  vaultKeyB64,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	SaveSessionMetadata(session)

	// Initialize the global vault with this session's key
	vault, err := privacy.NewVaultFromKey(keyBytes)
	if err == nil {
		privacy.ActiveVault = vault
	}

	return session, nil
}

// InitVault initializes the privacy vault from a loaded session's stored key
func InitVault(s *Session) error {
	if s.VaultKey == "" {
		// Legacy session without vault key; generate one
		keyBytes := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, keyBytes); err != nil {
			return err
		}
		s.VaultKey = base64.StdEncoding.EncodeToString(keyBytes)
		SaveSessionMetadata(s)
	}

	keyBytes, err := base64.StdEncoding.DecodeString(s.VaultKey)
	if err != nil {
		return fmt.Errorf("failed to decode vault key: %v", err)
	}

	vault, err := privacy.NewVaultFromKey(keyBytes)
	if err != nil {
		return err
	}

	privacy.ActiveVault = vault
	return nil
}

func SaveSessionMetadata(s *Session) error {
	metaFile := filepath.Join(config.GetConfigDir(), "sessions", s.ID, "metadata.json")
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaFile, data, 0644)
}

func LoadSessionMetadata(id string) (*Session, error) {
	metaFile := filepath.Join(config.GetConfigDir(), "sessions", id, "metadata.json")
	data, err := os.ReadFile(metaFile)
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func LogToSession(sessionID string, message string) error {
	today := time.Now().Format("2006-01-02")
	logFile := filepath.Join(config.GetConfigDir(), "sessions", sessionID, today+".md")

	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	timestamp := time.Now().Format("15:04:05")
	// In session logs: use tokenized (masked) form for security
	message = privacy.ApplyPrivacy(message)

	if _, err := f.WriteString(fmt.Sprintf("## [%s]\n\n%s\n\n", timestamp, message)); err != nil {
		return err
	}

	return nil
}

func ListSessions() ([]*Session, error) {
	sessionsDir := filepath.Join(config.GetConfigDir(), "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, err
	}

	var sessions []*Session
	for _, entry := range entries {
		if entry.IsDir() {
			s, err := LoadSessionMetadata(entry.Name())
			if err == nil {
				sessions = append(sessions, s)
			}
		}
	}

	// Sort by modification time
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

func ReadSessionLogs(sessionID string) (string, error) {
	sessionDir := filepath.Join(config.GetConfigDir(), "sessions", sessionID)
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return "", err
	}

	var fullLog strings.Builder
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".md" {
			content, err := os.ReadFile(filepath.Join(sessionDir, entry.Name()))
			if err == nil {
				fullLog.Write(content)
				fullLog.WriteString("\n\n---\n\n")
			}
		}
	}
	return fullLog.String(), nil
}

func UpdateSessionSummary(id string, summary string) error {
	s, err := LoadSessionMetadata(id)
	if err != nil {
		return err
	}
	s.Summary = summary
	s.UpdatedAt = time.Now()
	return SaveSessionMetadata(s)
}

func DeleteSession(id string) error {
	sessionDir := filepath.Join(config.GetConfigDir(), "sessions", id)
	return os.RemoveAll(sessionDir)
}

func ClearSessions() error {
	sessionsDir := filepath.Join(config.GetConfigDir(), "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			os.RemoveAll(filepath.Join(sessionsDir, entry.Name()))
		}
	}
	return nil
}
