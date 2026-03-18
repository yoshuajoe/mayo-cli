package privacy

import (
	"testing"
)

func TestDetector_Detect(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name     string
		text     string
		expected []EntityType
	}{
		{
			name:     "Email detection",
			text:     "Contact me at yoshua@example.com for info.",
			expected: []EntityType{EntityEmail},
		},
		{
			name:     "Phone detection",
			text:     "Call me at +62-812-3456-7890.",
			expected: []EntityType{EntityPhone},
		},
		{
			name:     "IP detection",
			text:     "The server IP is 192.168.1.1.",
			expected: []EntityType{EntityIPAddress},
		},
		{
			name:     "Mixed PII",
			text:     "User yoshua@example.com at 10.0.0.1 called from 081234455.",
			expected: []EntityType{EntityEmail, EntityIPAddress, EntityPhone},
		},
		{
			name:     "No PII",
			text:     "This is a regular text without any secrets.",
			expected: []EntityType{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entities := detector.Detect(tt.text)

			// Verify each expected entity type is found
			for _, expType := range tt.expected {
				found := false
				for _, ent := range entities {
					if ent.Type == expType {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected to find entity of type %v, but it was not detected", expType)
				}
			}

			// Verify no unexpected entities are found (simple check)
			if len(entities) > len(tt.expected) {
				// This might happen if multiple entities of same type exist, which is fine
				// But let's check unique types
				uniqueFound := make(map[EntityType]bool)
				for _, ent := range entities {
					uniqueFound[ent.Type] = true
				}
				if len(uniqueFound) > len(tt.expected) {
					t.Errorf("Detected more unique entity types than expected. Found: %v, Expected: %v", uniqueFound, tt.expected)
				}
			}
		})
	}
}

func TestToggleDetectInColumns(t *testing.T) {
	tests := []struct {
		colName  string
		expected bool
	}{
		{"email_address", true},
		{"user_phone", true},
		{"phone_number", true},
		{"nik", true},
		{"ktp_number", true},
		{"ip_addr", false}, // not explicitly in patterns
		{"user_id", false},
		{"created_at", false},
		{"credit_card", true},
		{"ssn", true},
	}

	for _, tt := range tests {
		got := DetectInColumns(tt.colName)
		if got != tt.expected {
			t.Errorf("DetectInColumns(%v) = %v; want %v", tt.colName, got, tt.expected)
		}
	}
}
