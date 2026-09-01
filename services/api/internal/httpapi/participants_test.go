package httpapi

import "testing"

func TestLogicalRealtimeIdentity(t *testing.T) {
	tests := map[string]string{
		"user-stable-id":                "user-stable-id",
		"user-id:participant-record-id": "user-id",
		"":                              "",
	}
	for input, expected := range tests {
		if actual := logicalRealtimeIdentity(input); actual != expected {
			t.Fatalf("logicalRealtimeIdentity(%q) = %q, want %q", input, actual, expected)
		}
	}
}
