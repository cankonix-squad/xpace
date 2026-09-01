package httpapi

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCreateDirectConversationReusesExistingConversation(t *testing.T) {
	api, mock := mockAPI(t)
	now := time.Now()
	mock.ExpectQuery("SELECT display_name FROM users").WithArgs("user-2", "tenant-1").WillReturnRows(sqlmock.NewRows([]string{"display_name"}).AddRow("Ciko"))
	mock.ExpectQuery("SELECT c.id,c.type::text,\\$4").WithArgs("tenant-1", "user-1", "user-2", "Ciko").WillReturnRows(sqlmock.NewRows([]string{"id", "type", "name", "created_at", "member_count", "unread_count", "online_count"}).AddRow("conversation-1", "DIRECT", "Ciko", now, 2, 0, 1))

	request := httptest.NewRequest(http.MethodPost, "/api/v1/chat/conversations", strings.NewReader(`{"type":"DIRECT","memberIds":["user-2"]}`))
	request.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	api.chatConversations(writer, request, currentUser{ID: "user-1", TenantID: "tenant-1"})

	if writer.Code != http.StatusOK || !strings.Contains(writer.Body.String(), `"existing":true`) || !strings.Contains(writer.Body.String(), `"name":"Ciko"`) {
		t.Fatalf("reuse direct conversation returned %d: %s", writer.Code, writer.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateDirectConversationRejectsInactiveOrForeignUser(t *testing.T) {
	api, mock := mockAPI(t)
	mock.ExpectQuery("SELECT display_name FROM users").WithArgs("user-2", "tenant-1").WillReturnError(sql.ErrNoRows)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/chat/conversations", strings.NewReader(`{"type":"DIRECT","memberIds":["user-2"]}`))
	request.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	api.chatConversations(writer, request, currentUser{ID: "user-1", TenantID: "tenant-1"})

	if writer.Code != http.StatusBadRequest || !strings.Contains(writer.Body.String(), "not active") {
		t.Fatalf("invalid direct member returned %d: %s", writer.Code, writer.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateChannelAddsSelectedWorkspaceMembers(t *testing.T) {
	api, mock := mockAPI(t)
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO chat_conversations").WithArgs("tenant-1", "CHANNEL", "Product team", "user-1").WillReturnRows(sqlmock.NewRows([]string{"id", "type", "name", "created_at"}).AddRow("conversation-2", "CHANNEL", "Product team", now))
	mock.ExpectExec("INSERT INTO chat_members").WithArgs("conversation-2", "tenant-1", "user-2").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO chat_members").WithArgs("conversation-2", "tenant-1", "user-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(0, 1))

	request := httptest.NewRequest(http.MethodPost, "/api/v1/chat/conversations", strings.NewReader(`{"type":"CHANNEL","name":"Product team","memberIds":["user-2"]}`))
	request.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	api.chatConversations(writer, request, currentUser{ID: "user-1", TenantID: "tenant-1"})

	if writer.Code != http.StatusCreated || !strings.Contains(writer.Body.String(), `"memberCount":2`) {
		t.Fatalf("create channel returned %d: %s", writer.Code, writer.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClearConversationOnlyClearsCurrentMemberHistory(t *testing.T) {
	api, mock := mockAPI(t)
	mock.ExpectExec("UPDATE chat_members SET cleared_at=NOW\\(\\),last_read_at=NOW\\(\\)").
		WithArgs("conversation-1", "tenant-1", "user-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(0, 1))

	request := httptest.NewRequest(http.MethodPost, "/api/v1/chat/conversations/conversation-1/clear", nil)
	request.SetPathValue("conversationID", "conversation-1")
	writer := httptest.NewRecorder()
	api.chatConversationClear(writer, request, currentUser{ID: "user-1", TenantID: "tenant-1"})

	if writer.Code != http.StatusNoContent {
		t.Fatalf("clear conversation returned %d: %s", writer.Code, writer.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteConversationSoftDeletesOnlyCurrentMembership(t *testing.T) {
	api, mock := mockAPI(t)
	mock.ExpectExec("UPDATE chat_members SET cleared_at=NOW\\(\\),hidden_at=NOW\\(\\),last_read_at=NOW\\(\\)").
		WithArgs("conversation-1", "tenant-1", "user-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(0, 1))

	request := httptest.NewRequest(http.MethodDelete, "/api/v1/chat/conversations/conversation-1", nil)
	request.SetPathValue("conversationID", "conversation-1")
	writer := httptest.NewRecorder()
	api.chatConversationDelete(writer, request, currentUser{ID: "user-1", TenantID: "tenant-1"})

	if writer.Code != http.StatusNoContent {
		t.Fatalf("delete conversation returned %d: %s", writer.Code, writer.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteConversationRejectsNonMember(t *testing.T) {
	api, mock := mockAPI(t)
	mock.ExpectExec("UPDATE chat_members SET cleared_at=NOW\\(\\),hidden_at=NOW\\(\\),last_read_at=NOW\\(\\)").
		WithArgs("conversation-1", "tenant-1", "user-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	request := httptest.NewRequest(http.MethodDelete, "/api/v1/chat/conversations/conversation-1", nil)
	request.SetPathValue("conversationID", "conversation-1")
	writer := httptest.NewRecorder()
	api.chatConversationDelete(writer, request, currentUser{ID: "user-1", TenantID: "tenant-1"})

	if writer.Code != http.StatusNotFound {
		t.Fatalf("delete non-member conversation returned %d: %s", writer.Code, writer.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
