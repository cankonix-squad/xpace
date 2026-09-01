package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJoinRequiresCurrentRecordingNotice(t *testing.T) {
	t.Setenv("RECORDING_CONSENT_VERSION", "2026-08-29")
	tests := []string{
		`{}`,
		`{"recordingNoticeAcknowledged":false,"recordingConsentVersion":"2026-08-29"}`,
		`{"recordingNoticeAcknowledged":true,"recordingConsentVersion":"old"}`,
	}
	for _, body := range tests {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/meetings/ABC-DEF-GHI/join", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		writer := httptest.NewRecorder()
		(&API{}).joinMeeting(writer, request, currentUser{})
		if writer.Code != http.StatusBadRequest || !bytes.Contains(writer.Body.Bytes(), []byte(`"code":"RECORDING_NOTICE_REQUIRED"`)) {
			t.Fatalf("expected recording notice rejection, got %d: %s", writer.Code, writer.Body.String())
		}
	}
}

func TestRecordingConsentVersionFallback(t *testing.T) {
	t.Setenv("RECORDING_CONSENT_VERSION", "")
	if currentRecordingConsentVersion() != "2026-08-29" {
		t.Fatal("unexpected recording consent fallback version")
	}
}
