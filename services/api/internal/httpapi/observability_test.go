package httpapi

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/prometheus/client_golang/prometheus"
)

func TestPlatformMetricsCollectorExportsOperationalSnapshot(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT")).WillReturnRows(sqlmock.NewRows([]string{
		"active_meetings", "waiting_participants", "joined_participants", "failed_recordings",
		"drive_storage", "chat_storage", "recording_storage", "chat_messages", "client_errors",
	}).AddRow(2, 3, 4, 1, 100, 200, 300, 5, 6))

	registry := prometheus.NewRegistry()
	registry.MustRegister(newPlatformMetricsCollector(database))
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{
		"xpace_database_connections":          false,
		"xpace_platform_active_meetings":      false,
		"xpace_platform_storage_bytes":        false,
		"xpace_platform_metrics_scrape_error": false,
	}
	for _, family := range families {
		if _, exists := wanted[family.GetName()]; exists {
			wanted[family.GetName()] = true
		}
	}
	for name, found := range wanted {
		if !found {
			t.Errorf("metric family %s was not exported", name)
		}
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
