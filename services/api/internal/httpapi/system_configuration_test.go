package httpapi

import "testing"

func TestSystemConfigurationValidation(t *testing.T) {
	valid := systemConfiguration{WorkspaceName: "Cankonix", DefaultTimezone: "Asia/Jakarta", DefaultLocale: "id-ID", SupportEmail: "support@example.com", MaxMeetingDurationMinutes: 120, RecordingRetentionDays: 30}
	if code, message := valid.validate(); code != "" {
		t.Fatalf("valid configuration rejected: %s %s", code, message)
	}
	tests := []struct {
		name string
		edit func(*systemConfiguration)
		code string
	}{
		{"timezone", func(value *systemConfiguration) { value.DefaultTimezone = "Mars/Olympus" }, "INVALID_TIMEZONE"},
		{"locale", func(value *systemConfiguration) { value.DefaultLocale = "id_Indonesia" }, "INVALID_LOCALE"},
		{"email", func(value *systemConfiguration) { value.SupportEmail = "invalid" }, "INVALID_SUPPORT_EMAIL"},
		{"duration", func(value *systemConfiguration) { value.MaxMeetingDurationMinutes = 5 }, "INVALID_MEETING_DURATION"},
		{"retention", func(value *systemConfiguration) { value.RecordingRetentionDays = 0 }, "INVALID_RECORDING_RETENTION"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.edit(&candidate)
			if code, _ := candidate.validate(); code != test.code {
				t.Fatalf("got %q, want %q", code, test.code)
			}
		})
	}
}
