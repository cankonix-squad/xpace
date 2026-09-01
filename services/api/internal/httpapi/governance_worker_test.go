package httpapi

import (
	"testing"
	"time"
)

func TestGovernanceWorkerConfigDefaultsAndOverrides(t *testing.T) {
	t.Setenv("GOVERNANCE_RETENTION_WORKER_ENABLED", "")
	t.Setenv("GOVERNANCE_RETENTION_INITIAL_DELAY", "")
	t.Setenv("GOVERNANCE_RETENTION_INTERVAL", "")
	config := loadGovernanceWorkerConfig()
	if !config.Enabled || config.InitialDelay != time.Minute || config.Interval != 24*time.Hour {
		t.Fatalf("unexpected defaults: %+v", config)
	}
	t.Setenv("GOVERNANCE_RETENTION_WORKER_ENABLED", "false")
	t.Setenv("GOVERNANCE_RETENTION_INITIAL_DELAY", "30s")
	t.Setenv("GOVERNANCE_RETENTION_INTERVAL", "6h")
	config = loadGovernanceWorkerConfig()
	if config.Enabled || config.InitialDelay != 30*time.Second || config.Interval != 6*time.Hour {
		t.Fatalf("unexpected overrides: %+v", config)
	}
}

func TestGovernanceWorkerConfigRejectsUnsafeIntervals(t *testing.T) {
	t.Setenv("GOVERNANCE_RETENTION_INITIAL_DELAY", "1s")
	t.Setenv("GOVERNANCE_RETENTION_INTERVAL", "1m")
	config := loadGovernanceWorkerConfig()
	if config.InitialDelay != time.Minute || config.Interval != 24*time.Hour {
		t.Fatalf("unsafe values should fall back to defaults: %+v", config)
	}
}
