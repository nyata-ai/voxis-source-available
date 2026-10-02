package service

import (
	"fmt"
	"regexp"

	"github.com/voxis/backend/internal/domain"
)

var (
	emailRegex   = regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`)
	urlRegex     = regexp.MustCompile(`https?://\S+`)
	phoneRegex   = regexp.MustCompile(`(?:\+?\d[\d\s().-]{7,}\d)`)
	accountRegex = regexp.MustCompile(`\b\d{10,18}\b`)
	caseIDRegex  = regexp.MustCompile(`\b[A-Z]{1,4}/\d{2,6}/[A-Z0-9]{1,6}/\d{4}\b`)
)

// RedactionResult contains deterministic redacted text and metadata.
type RedactionResult struct {
	Text       string   `json:"text"`
	Categories []string `json:"redacted_categories"`
	Warnings   []string `json:"redaction_warnings,omitempty"`
}

// RedactText applies deterministic v1 redaction policies. An unknown policy
// name, or "education" on its own (it has no rules yet), fails with
// domain.ErrInvalidInput rather than returning unredacted text. Text with no
// matches under a known policy is returned unchanged, which is correct.
func RedactText(text string, policies []string) (RedactionResult, error) {
	if len(policies) == 0 {
		policies = []string{"pii"}
	}
	if err := validateRedactionPolicies(policies); err != nil {
		return RedactionResult{}, err
	}

	result := RedactionResult{Text: text}
	seen := make(map[string]bool, len(policies))
	for _, policy := range policies {
		if seen[policy] {
			continue
		}
		seen[policy] = true
		switch policy {
		case "pii":
			result.Text = emailRegex.ReplaceAllString(result.Text, "[REDACTED_EMAIL]")
			result.Text = urlRegex.ReplaceAllString(result.Text, "[REDACTED_URL]")
			result.Text = phoneRegex.ReplaceAllString(result.Text, "[REDACTED_PHONE]")
			result.Categories = append(result.Categories, policy)
		case "financial":
			result.Text = accountRegex.ReplaceAllString(result.Text, "[REDACTED_ACCOUNT]")
			result.Categories = append(result.Categories, policy)
		case "law_enforcement":
			result.Text = caseIDRegex.ReplaceAllString(result.Text, "[REDACTED_CASE_ID]")
			result.Categories = append(result.Categories, policy)
		case "education":
			result.Categories = append(result.Categories, policy)
			result.Warnings = append(result.Warnings, "education policy has no deterministic v1 rules")
		}
	}
	return result, nil
}

// validateRedactionPolicies rejects unknown names and a policy set that
// would redact nothing.
func validateRedactionPolicies(policies []string) error {
	hasRules := false
	for _, policy := range policies {
		switch policy {
		case "pii", "financial", "law_enforcement":
			hasRules = true
		case "education":
		default:
			return fmt.Errorf("unsupported redaction policy %q; use pii, financial, law_enforcement, or education: %w",
				policy, domain.ErrInvalidInput)
		}
	}
	if !hasRules {
		return fmt.Errorf("the education redaction policy has no rules of its own; combine it with pii, financial, or law_enforcement: %w",
			domain.ErrInvalidInput)
	}
	return nil
}
