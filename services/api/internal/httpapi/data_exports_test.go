package httpapi

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestValidDataExportType(t *testing.T) {
	for _, value := range []string{"FULL", " audit ", "directory"} {
		if !validDataExportType(value) {
			t.Fatalf("expected %q to be valid", value)
		}
	}
	for _, value := range []string{"", "PASSWORDS", "CHAT"} {
		if validDataExportType(value) {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}

func TestDataExportFourEyesRejectsRequesterApproval(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	api := &API{database: database}
	mock.ExpectExec("UPDATE data_export_requests").WithArgs("APPROVED", "admin", "", "export", "tenant").WillReturnResult(sqlmock.NewResult(0, 0))
	request := httptest.NewRequest("POST", "/api/v1/admin/governance/exports/export/approve", bytes.NewBufferString(`{"note":""}`))
	request.SetPathValue("exportID", "export")
	response := httptest.NewRecorder()
	api.reviewDataExport(response, request, currentUser{ID: "admin", TenantID: "tenant", Role: roleTenantAdmin})
	if response.Code != 409 {
		t.Fatalf("status = %d, want 409", response.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDataExportRejectRequiresReason(t *testing.T) {
	database, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	api := &API{database: database}
	request := httptest.NewRequest("POST", "/api/v1/admin/governance/exports/export/reject", bytes.NewBufferString(`{"note":"no"}`))
	request.SetPathValue("exportID", "export")
	response := httptest.NewRecorder()
	api.reviewDataExport(response, request, currentUser{ID: "reviewer", TenantID: "tenant", Role: roleTenantAdmin})
	if response.Code != 400 {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}
