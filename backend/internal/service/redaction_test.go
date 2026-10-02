package service

import (
	"errors"
	"testing"

	"github.com/voxis/backend/internal/domain"
)

func TestRedactTextRejectsPoliciesThatWouldNotRedact(t *testing.T) {
	for _, policies := range [][]string{{"PII"}, {"education"}, {"pii", "medical"}, {"education", "education"}} {
		result, err := RedactText("Call ada@example.com", policies)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("RedactText(%q) error = %v, want ErrInvalidInput", policies, err)
		}
		if result.Text != "" {
			t.Fatalf("RedactText(%q) returned text %q with an error", policies, result.Text)
		}
	}
}

func TestRedactTextAppliesKnownPolicies(t *testing.T) {
	result, err := RedactText("Call ada@example.com", nil)
	if err != nil || result.Text != "Call [REDACTED_EMAIL]" {
		t.Fatalf("default policy = %q, %v", result.Text, err)
	}
	result, err = RedactText("Account 1234567890", []string{"education", "financial"})
	if err != nil || result.Text != "Account [REDACTED_ACCOUNT]" || len(result.Warnings) != 1 {
		t.Fatalf("education with financial = %#v, %v", result, err)
	}
}

func TestRedactTextWithoutMatchesIsNotAnError(t *testing.T) {
	result, err := RedactText("Nothing sensitive here", []string{"pii", "law_enforcement"})
	if err != nil || result.Text != "Nothing sensitive here" {
		t.Fatalf("RedactText() = %#v, %v", result, err)
	}
}
