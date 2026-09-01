package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAdminGroupsDecodesJSONMemberIDs(t *testing.T) {
	api, mock := mockAPI(t)
	mock.ExpectQuery("JSON_AGG").WithArgs("tenant-1").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "member_count", "member_ids"}).
		AddRow("group-1", "Engineering", "Product engineering", 2, []byte(`["user-1","user-2"]`)).
		AddRow("group-2", "Finance", "", 0, []byte(`[]`)))

	writer := httptest.NewRecorder()
	api.adminGroups(writer, httptest.NewRequest(http.MethodGet, "/api/v1/admin/groups", nil), currentUser{ID: "admin-1", TenantID: "tenant-1", Role: roleSuperAdmin})

	if writer.Code != http.StatusOK || !strings.Contains(writer.Body.String(), `"memberIds":["user-1","user-2"]`) || !strings.Contains(writer.Body.String(), `"memberIds":[]`) {
		t.Fatalf("groups returned %d: %s", writer.Code, writer.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdminGroupsRejectsMalformedMemberAggregation(t *testing.T) {
	api, mock := mockAPI(t)
	mock.ExpectQuery("JSON_AGG").WithArgs("tenant-1").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "member_count", "member_ids"}).AddRow("group-1", "Engineering", "", 1, []byte(`invalid`)))

	writer := httptest.NewRecorder()
	api.adminGroups(writer, httptest.NewRequest(http.MethodGet, "/api/v1/admin/groups", nil), currentUser{ID: "admin-1", TenantID: "tenant-1", Role: roleSuperAdmin})

	if writer.Code != http.StatusInternalServerError {
		t.Fatalf("invalid aggregation returned %d: %s", writer.Code, writer.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
