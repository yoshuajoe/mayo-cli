package privacy

import (
	"regexp"
	"strings"
)

var (
	// Credential Patterns (always masked, not tokenized)
	dsnRegex    = regexp.MustCompile(`([a-zA-Z0-9+.-]+://)([^:]+):([^@]+)(@)`)
	apiKeyRegex = regexp.MustCompile(`(?i)(api[-_]?key|secret|password|token|auth|dsn)[:=]\s*["']?([a-zA-Z0-9\-_./+=]{8,})["']?`)
)

// MaskCredentials always hides DSNs and API keys (never sent to AI)
func MaskCredentials(text string) string {
	text = dsnRegex.ReplaceAllString(text, "$1$2:****$4")
	text = apiKeyRegex.ReplaceAllString(text, "$1: [MASKED]")
	return text
}

// Global Privacy Toggle
var PrivacyMode = true

// Global Vault (set per session)
var ActiveVault *Vault

// ApplyPrivacy masks credentials.
// When Vault is active, it also tokenizes PII for AI-bound text.
func ApplyPrivacy(text string) string {
	// Always mask credentials regardless of privacy mode
	text = MaskCredentials(text)
	if !PrivacyMode {
		return text
	}
	// If vault is active, use tokenization
	if ActiveVault != nil {
		return ActiveVault.Tokenize(text)
	}
	// Fallback: simple masking (legacy behavior)
	return simpleMaskPII(text)
}

// RestorePrivacy detokenizes text back to original values (for displaying to user)
func RestorePrivacy(text string) string {
	if ActiveVault == nil {
		return text
	}
	return ActiveVault.Detokenize(text)
}

// simpleMaskPII is the legacy/fallback masking function
func simpleMaskPII(text string) string {
	emailRe := regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	text = emailRe.ReplaceAllStringFunc(text, func(m string) string {
		parts := strings.Split(m, "@")
		if len(parts) != 2 {
			return m
		}
		username := parts[0]
		if len(username) > 1 {
			return username[:1] + "****@" + parts[1]
		}
		return "****@" + parts[1]
	})
	return text
}
