package privacy

import (
	"regexp"
)

// EntityType represents the type of PII detected
type EntityType string

const (
	EntityEmail      EntityType = "EMAIL"
	EntityPhone      EntityType = "PHONE"
	EntityCreditCard EntityType = "CARD"
	EntityUUID       EntityType = "UUID"
	EntityIPAddress  EntityType = "IP"
	EntityURL        EntityType = "URL"
	EntitySSN        EntityType = "SSN"
	EntityOrg        EntityType = "ORG"
)

// DetectedEntity represents a single detected PII entity
type DetectedEntity struct {
	Type     EntityType
	Value    string
	StartIdx int
	EndIdx   int
}

// Detector holds compiled regex patterns for entity detection
type Detector struct {
	patterns map[EntityType]*regexp.Regexp
}

// NewDetector creates a new PII detector with precompiled patterns
func NewDetector() *Detector {
	return &Detector{
		patterns: map[EntityType]*regexp.Regexp{
			// Email: standard email pattern
			EntityEmail: regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`),

			// Phone: international formats (+62xxx, 08xxx, etc.)
			EntityPhone: regexp.MustCompile(`(?:(?:\+|00)\d{1,4}[\s\-]?)?\(?\d{2,4}\)?[\s\-]?\d{3,4}[\s\-]?\d{3,6}`),

			// Credit Card: 4 groups of 4 digits or 16 consecutive digits
			EntityCreditCard: regexp.MustCompile(`\b(?:\d{4}[\s\-]?){3}\d{4}\b`),

			// UUID: standard UUID format (v4 and others)
			EntityUUID: regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`),

			// IP Address: IPv4
			EntityIPAddress: regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),

			// SSN: US Social Security Number
			EntitySSN: regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),

			// Organizations: PT, CV, Inc., LLC, Corp., GmbH
			EntityOrg: regexp.MustCompile(`(?i)\b(PT|CV|Inc|LLC|Corp|GmbH|Ltd|Co)\.?\s+[A-Z][A-Za-z0-9&.\-]{2,}(?:\s+[A-Za-z0-9&.\-]{1,})*\b|\b[A-Z][A-Za-z0-9&.\-]{2,}(?:\s+[A-Za-z0-9&.\-]{1,})*\s+(PT|CV|Inc|LLC|Corp|GmbH|Ltd|Co)\.?\b`),
		},
	}
}

// Detect scans text for all known PII entities and returns them
func (d *Detector) Detect(text string) []DetectedEntity {
	var entities []DetectedEntity
	seen := make(map[string]bool) // Deduplicate

	for entityType, pattern := range d.patterns {
		matches := pattern.FindAllStringIndex(text, -1)
		for _, match := range matches {
			value := text[match[0]:match[1]]

			// Skip if too short (avoid false positives)
			if len(value) < 5 {
				continue
			}

			// Skip known false positives for IPs (e.g., version numbers like 1.2.3)
			if entityType == EntityIPAddress && len(value) < 7 {
				continue
			}

			key := string(entityType) + ":" + value
			if seen[key] {
				continue
			}
			seen[key] = true

			entities = append(entities, DetectedEntity{
				Type:     entityType,
				Value:    value,
				StartIdx: match[0],
				EndIdx:   match[1],
			})
		}
	}

	return entities
}

// DetectInColumns checks a column name against known sensitive patterns
// and returns true if the column likely contains PII data
func DetectInColumns(colName string) bool {
	sensitivePatterns := regexp.MustCompile(`(?i)(email|phone|mobile|password|secret|token|ssn|credit.?card|card.?number|address|birth|dob|passport|national.?id|nik|ktp|npwp)`)
	return sensitivePatterns.MatchString(colName)
}
