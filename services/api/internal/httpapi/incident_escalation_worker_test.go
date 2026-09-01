package httpapi

import (
	"testing"
	"time"
)

func TestIncidentEscalationWorkerConfig(t *testing.T) {
	t.Setenv("INCIDENT_ESCALATION_INITIAL_DELAY", "10s")
	t.Setenv("INCIDENT_ESCALATION_INTERVAL", "30s")
	config := loadIncidentEscalationWorkerConfig()
	if !config.Enabled || config.InitialDelay != 10*time.Second || config.Interval != 30*time.Second {
		t.Fatalf("unexpected config: %#v", config)
	}
	t.Setenv("INCIDENT_ESCALATION_WORKER_ENABLED", "false")
	if loadIncidentEscalationWorkerConfig().Enabled {
		t.Fatal("worker must honor disabled configuration")
	}
}
