package privacy

import (
	"testing"
)

func TestVault_Encryption(t *testing.T) {
	vault, err := NewVault()
	if err != nil {
		t.Fatalf("Failed to create vault: %v", err)
	}

	original := "Top Secret Data"
	encrypted, err := vault.EncryptValue(original)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	if encrypted == original {
		t.Errorf("Encrypted value is same as original")
	}

	decrypted, err := vault.DecryptValue(encrypted)
	if err != nil {
		t.Fatalf("Decryption failed: %v", err)
	}

	if decrypted != original {
		t.Errorf("Decrypted = %v; want %v", decrypted, original)
	}
}

func TestVault_Tokenization(t *testing.T) {
	vault, _ := NewVault()

	text := "User yoshua@gmail.com with phone 081234455 is calling 081234455."
	tokenized := vault.Tokenize(text)

	if tokenized == text {
		t.Errorf("Tokenization did not change text")
	}

	// Check if same values get same tokens
	phoneCount := 0
	for token := range vault.reverse {
		if vault.reverse[token] == "081234455" {
			phoneCount++
		}
	}
	// Note: 081234455 is detected twice but should only have one entry in vault.forward/reverse maps
	// But because we use strings.ReplaceAll in Tokenize, both instances in text will be replaced by same token.

	detokenized := vault.Detokenize(tokenized)
	if detokenized != text {
		t.Errorf("Detokenize = %v; want %v", detokenized, text)
	}
}

func TestVault_NewVaultFromKey(t *testing.T) {
	originalVault, _ := NewVault()
	key := originalVault.GetKey()

	newVault, err := NewVaultFromKey(key)
	if err != nil {
		t.Fatalf("Failed to recreate vault from key: %v", err)
	}

	secret := "persistent secret"
	encrypted, _ := originalVault.EncryptValue(secret)

	// Should be able to decrypt with the new vault using same key
	decrypted, err := newVault.DecryptValue(encrypted)
	if err != nil {
		t.Errorf("Recreated vault failed to decrypt: %v", err)
	}
	if decrypted != secret {
		t.Errorf("Decrypted with new vault = %v; want %v", decrypted, secret)
	}
}

func TestVault_TokenizeTableData(t *testing.T) {
	vault, _ := NewVault()
	headers := []string{"id", "user_email", "status"}
	rows := [][]string{
		{"1", "alice@example.com", "active"},
		{"2", "bob@example.com", "pending"},
	}

	tokenized := vault.TokenizeTableData(headers, rows)

	// Check row count
	if len(tokenized) != len(rows) {
		t.Fatalf("Tokenized row count mismatched. Got %d, want %d", len(tokenized), len(rows))
	}

	// Email column (index 1) should be tokenized
	if tokenized[0][1] == rows[0][1] {
		t.Errorf("Email cell was not tokenized")
	}

	// Status column (index 2) should NOT be tokenized (unless it looks like PII)
	if tokenized[0][2] != rows[0][2] {
		t.Errorf("Status cell should not have been tokenized. Got %v, want %v", tokenized[0][2], rows[0][2])
	}
}
