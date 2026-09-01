package httpapi

import (
	"strings"
	"testing"
)

func TestSignupIdentityValidation(t *testing.T) {
	validEmails := []string{"owner@example.com", "owner+test@example.co.id"}
	invalidEmails := []string{"", "owner", "Owner <owner@example.com>", "owner @example.com"}
	for _, value := range validEmails {
		if !validSignupEmail(value) {
			t.Errorf("expected valid email %q", value)
		}
	}
	for _, value := range invalidEmails {
		if validSignupEmail(value) {
			t.Errorf("expected invalid email %q", value)
		}
	}
	validUsernames := []string{"owner", "ciko.test", "team_admin-1"}
	invalidUsernames := []string{"a", "owner name", "Owner", "owner@example"}
	for _, value := range validUsernames {
		if !validUsername(value) {
			t.Errorf("expected valid username %q", value)
		}
	}
	for _, value := range invalidUsernames {
		if validUsername(value) {
			t.Errorf("expected invalid username %q", value)
		}
	}
}

func TestTransactionalEmailTemplates(t *testing.T) {
	tests := []struct {
		template, token string
		payload         map[string]any
		path            string
	}{
		{"VERIFY_EMAIL", "verify-token", map[string]any{"publicUrl": "https://xspace.example"}, "/verify-email?token=verify-token"},
		{"RESET_PASSWORD", "reset-token", map[string]any{"publicUrl": "https://xspace.example"}, "/reset-password?token=reset-token"},
		{"INVITATION", "invite-token", map[string]any{"publicUrl": "https://xspace.example", "workspace": "Cankonix", "inviter": "Admin"}, "/accept-invite?token=invite-token"},
		{"BILLING_NOTICE", "", map[string]any{"publicUrl": "https://xspace.example", "event": "invoice.paid", "message": "Invoice paid."}, "/admin/billing"},
		{"SECURITY_NOTICE", "", map[string]any{"publicUrl": "https://xspace.example", "event": "MFA enabled", "message": "MFA enabled."}, "/security"},
	}
	for _, test := range tests {
		content, err := transactionalEmail(test.template, test.token, test.payload)
		if err != nil {
			t.Fatalf("%s: %v", test.template, err)
		}
		if content.Subject == "" || !strings.Contains(content.Link, test.path) {
			t.Fatalf("invalid %s content: %+v", test.template, content)
		}
		textBody, htmlBody := renderTransactionalEmail(content)
		if !strings.Contains(textBody, content.Link) || !strings.Contains(htmlBody, content.Link) || !strings.Contains(htmlBody, "Xspace") {
			t.Fatalf("%s did not render both email bodies", test.template)
		}
	}
}

func TestMailConfigurationRequiresDeliveryFields(t *testing.T) {
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_FROM", "")
	if _, configured := loadMailConfig(); configured {
		t.Fatal("empty SMTP configuration must be disabled")
	}
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_FROM", "Xpace <noreply@example.com>")
	if config, configured := loadMailConfig(); !configured || config.port != 587 {
		t.Fatalf("expected configured SMTP, configured=%v port=%d", configured, config.port)
	}
}
