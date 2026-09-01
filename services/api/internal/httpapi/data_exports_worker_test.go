package httpapi

import (
	"strings"
	"testing"
	"time"
)

func TestDataExportWorkerConfig(t *testing.T) {
	t.Setenv("DATA_EXPORT_WORKER_ENABLED", "")
	t.Setenv("DATA_EXPORT_WORKER_INITIAL_DELAY", "")
	t.Setenv("DATA_EXPORT_WORKER_INTERVAL", "")
	config := loadDataExportWorkerConfig()
	if !config.Enabled || config.InitialDelay != 15*time.Second || config.Interval != 30*time.Second {
		t.Fatalf("unexpected defaults: %+v", config)
	}
	t.Setenv("DATA_EXPORT_WORKER_ENABLED", "0")
	t.Setenv("DATA_EXPORT_WORKER_INITIAL_DELAY", "10s")
	t.Setenv("DATA_EXPORT_WORKER_INTERVAL", "45s")
	config = loadDataExportWorkerConfig()
	if config.Enabled || config.InitialDelay != 10*time.Second || config.Interval != 45*time.Second {
		t.Fatalf("unexpected overrides: %+v", config)
	}
}

func TestCollectionsForDataExportExcludeSecrets(t *testing.T) {
	collections := collectionsForDataExport("FULL")
	if len(collections) < 10 {
		t.Fatalf("full export has too few collections: %d", len(collections))
	}
	for _, collection := range collections {
		if strings.Contains(collection.Query, "password_hash") && !strings.Contains(collection.Query, "-'password_hash'") {
			t.Fatalf("collection %s may expose password hashes", collection.Name)
		}
	}
}
