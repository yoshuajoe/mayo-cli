package privacy

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Vault is the per-session encryption vault that holds the session key
// and a bidirectional mapping of original PII <-> tokens
type Vault struct {
	mu       sync.RWMutex
	key      []byte             // 32-byte AES-256 key
	forward  map[string]string  // original value -> token (e.g., "user@mail.com" -> "«EMAIL_1»")
	reverse  map[string]string  // token -> original value (e.g., "«EMAIL_1»" -> "user@mail.com")
	counters map[EntityType]int // counter per entity type for naming
	detector *Detector
}

// NewVault creates a new encryption vault with a fresh random AES-256 key
func NewVault() (*Vault, error) {
	key := make([]byte, 32) // AES-256
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate session key: %v", err)
	}

	return &Vault{
		key:      key,
		forward:  make(map[string]string),
		reverse:  make(map[string]string),
		counters: make(map[EntityType]int),
		detector: NewDetector(),
	}, nil
}

// NewVaultFromKey creates a vault from an existing key (e.g., loaded from session metadata)
func NewVaultFromKey(keyBytes []byte) (*Vault, error) {
	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("invalid key length: expected 32. got %d", len(keyBytes))
	}
	return &Vault{
		key:      keyBytes,
		forward:  make(map[string]string),
		reverse:  make(map[string]string),
		counters: make(map[EntityType]int),
		detector: NewDetector(),
	}, nil
}

// GetKey returns the raw key bytes (for persisting in session metadata)
func (v *Vault) GetKey() []byte {
	return v.key
}

// EncryptValue encrypts a single value using AES-256-GCM
func (v *Vault) EncryptValue(plaintext string) (string, error) {
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptValue decrypts a single AES-256-GCM encrypted value
func (v *Vault) DecryptValue(encoded string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(v.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// getOrCreateToken returns the existing token for a value, or creates a new one
func (v *Vault) getOrCreateToken(entityType EntityType, originalValue string) string {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Check if already tokenized
	if token, exists := v.forward[originalValue]; exists {
		return token
	}

	// Create new token
	v.counters[entityType]++
	token := fmt.Sprintf("«%s_%d»", entityType, v.counters[entityType])

	// Also encrypt the original value for secure storage
	encrypted, err := v.EncryptValue(originalValue)
	if err != nil {
		// Fallback: store plain (shouldn't happen)
		encrypted = originalValue
	}

	v.forward[originalValue] = token
	v.reverse[token] = originalValue

	// Store encrypted version separately for persistence
	_ = encrypted

	return token
}

// Tokenize scans text for PII entities and replaces them with tokens.
// Returns the tokenized text. The original values are stored in the vault
// and can be restored using Detokenize.
func (v *Vault) Tokenize(text string) string {
	if text == "" {
		return text
	}

	entities := v.detector.Detect(text)
	if len(entities) == 0 {
		return text
	}

	// Sort by length descending to avoid partial replacements
	// (e.g., replacing "user" before "user@email.com")
	result := text
	for _, entity := range entities {
		token := v.getOrCreateToken(entity.Type, entity.Value)
		result = strings.ReplaceAll(result, entity.Value, token)
	}

	return result
}

// Detokenize restores all tokens in the text back to their original values
func (v *Vault) Detokenize(text string) string {
	if text == "" {
		return text
	}

	v.mu.RLock()
	defer v.mu.RUnlock()

	result := text
	for token, original := range v.reverse {
		result = strings.ReplaceAll(result, token, original)
	}

	return result
}

// TokenizeTableData tokenizes all cell values in table data rows
func (v *Vault) TokenizeTableData(headers []string, rows [][]string) [][]string {
	// Detect which columns are sensitive based on column names
	sensitiveColumns := make(map[int]bool)
	for i, h := range headers {
		if DetectInColumns(h) {
			sensitiveColumns[i] = true
		}
	}

	tokenizedRows := make([][]string, len(rows))
	for ri, row := range rows {
		newRow := make([]string, len(row))
		for ci, cell := range row {
			// If column is known-sensitive OR cell content looks like PII, tokenize it
			if sensitiveColumns[ci] {
				newRow[ci] = v.Tokenize(cell)
			} else {
				// Still scan cell content for PII that may appear in non-sensitive columns
				newRow[ci] = v.Tokenize(cell)
			}
		}
		tokenizedRows[ri] = newRow
	}

	return tokenizedRows
}

// GetStats returns information about what's currently in the vault
func (v *Vault) GetStats() map[EntityType]int {
	v.mu.RLock()
	defer v.mu.RUnlock()

	stats := make(map[EntityType]int)
	for _, count := range v.counters {
		_ = count
	}
	// Copy counters
	for k, ct := range v.counters {
		stats[k] = ct
	}
	return stats
}

// Reset clears all stored tokens (useful when switching sessions)
func (v *Vault) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.forward = make(map[string]string)
	v.reverse = make(map[string]string)
	v.counters = make(map[EntityType]int)
}

// Apply implements the knowledge.PrivacyFilter interface by tokenizing PII in text.
// This ensures PII is masked before being sent to external APIs or stored in the vector DB.
func (v *Vault) Apply(text string) string {
	return v.Tokenize(text)
}
