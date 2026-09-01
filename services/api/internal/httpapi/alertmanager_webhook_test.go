package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAlertmanagerWebhookRejectsMissingToken(t *testing.T) {
	t.Setenv("ALERTMANAGER_WEBHOOK_SECRET", strings.Repeat("s", 32))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/alertmanager", strings.NewReader(`{"alerts":[]}`))
	writer := httptest.NewRecorder()
	(&API{}).alertmanagerWebhook(writer, request)
	if writer.Code != http.StatusUnauthorized {
		t.Fatalf("missing webhook token returned %d", writer.Code)
	}
}

func TestAlertIncidentSeverityMapping(t *testing.T) {
	tests := map[string]string{"critical": "P1", "warning": "P2", "unknown": "P3", "info": "P4"}
	for input, wanted := range tests {
		if got := alertIncidentSeverity(input); got != wanted {
			t.Errorf("severity %q mapped to %s, want %s", input, got, wanted)
		}
	}
}

func TestCleanAlertTextBoundsInput(t *testing.T) {
	if got := cleanAlertText("  incident  ", 20); got != "incident" {
		t.Fatalf("unexpected cleaned text %q", got)
	}
	if got := cleanAlertText("123456", 4); got != "1234" {
		t.Fatalf("unexpected bounded text %q", got)
	}
}
