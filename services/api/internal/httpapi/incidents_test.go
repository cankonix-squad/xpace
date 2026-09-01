package httpapi

import "testing"

func TestIncidentTransitionStateMachine(t *testing.T) {
	tests := []struct {
		action, target, event, allowed string
	}{
		{"acknowledge", "ACKNOWLEDGED", "ACKNOWLEDGED", "OPEN"},
		{"investigate", "INVESTIGATING", "INVESTIGATING", "ACKNOWLEDGED"},
		{"resolve", "RESOLVED", "RESOLVED", "INVESTIGATING"},
		{"close", "CLOSED", "CLOSED", "RESOLVED"},
		{"reopen", "OPEN", "REOPENED", "CLOSED"},
	}
	for _, test := range tests {
		target, event, allowed := incidentTransition(test.action)
		if target != test.target || event != test.event || !allowed[test.allowed] {
			t.Errorf("unexpected transition for %s: %s %s %#v", test.action, target, event, allowed)
		}
	}
	if target, _, _ := incidentTransition("delete"); target != "" {
		t.Fatal("unsupported transition must be rejected")
	}
}

func TestIncidentValidationCatalog(t *testing.T) {
	for _, severity := range []string{"P1", "P2", "P3", "P4"} {
		if !validIncidentSeverity(severity) {
			t.Errorf("severity %s must be valid", severity)
		}
	}
	if validIncidentSeverity("P0") || validIncidentStatus("DELETED") || validIncidentSource("WEBHOOK") {
		t.Fatal("unknown incident catalog values must be rejected")
	}
}
