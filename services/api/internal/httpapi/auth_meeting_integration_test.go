//go:build integration

package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"testing"
	"time"

	passwordauth "github.com/cankonix/xpace/api/internal/auth"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestIntegrationAuthMeetingLifecycle(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}
	baseURL := strings.TrimRight(envOr("XPACE_TEST_API_URL", "http://127.0.0.1:8080"), "/")
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err = database.PingContext(ctx); err != nil {
		t.Fatalf("postgres unavailable: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantSlug := "integration-" + suffix
	var tenantID, hostID, memberID string
	password := "Integration!Pass#2026"
	hash, err := passwordauth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	err = database.QueryRowContext(ctx, `INSERT INTO tenants (slug,name) VALUES ($1,'Integration Test') RETURNING id`, tenantSlug).Scan(&tenantID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupTenant(t, database, tenantID) })
	err = database.QueryRowContext(ctx, `INSERT INTO users (tenant_id,email,username,display_name,password_hash,role,status) VALUES ($1,$2,'host','Integration Host',$3,'HOST','ACTIVE') RETURNING id`, tenantID, "host-"+suffix+"@test.invalid", hash).Scan(&hostID)
	if err == nil {
		err = database.QueryRowContext(ctx, `INSERT INTO users (tenant_id,email,username,display_name,password_hash,role,status) VALUES ($1,$2,'member','Integration Member',$3,'MEMBER','ACTIVE') RETURNING id`, tenantID, "member-"+suffix+"@test.invalid", hash).Scan(&memberID)
	}
	if err != nil {
		t.Fatal(err)
	}

	host := newAPIClient(t)
	member := newAPIClient(t)
	doJSON(t, host, http.MethodPost, baseURL+"/api/v1/auth/login", map[string]any{"tenant": tenantSlug, "identity": "host", "password": password}, http.StatusOK, nil)
	doJSON(t, member, http.MethodPost, baseURL+"/api/v1/auth/login", map[string]any{"tenant": tenantSlug, "identity": "member", "password": password}, http.StatusOK, nil)
	doJSON(t, member, http.MethodPost, baseURL+"/api/v1/auth/login", map[string]any{"tenant": tenantSlug, "identity": "member", "password": "wrong-password"}, http.StatusUnauthorized, nil)

	var created struct {
		Meeting struct {
			ID       string `json:"id"`
			JoinCode string `json:"joinCode"`
		} `json:"meeting"`
	}
	doJSON(t, host, http.MethodPost, baseURL+"/api/v1/meetings", map[string]any{"title": "Integration Lifecycle"}, http.StatusCreated, &created)
	if created.Meeting.ID == "" || created.Meeting.JoinCode == "" {
		t.Fatal("meeting identifiers were not returned")
	}
	meetingURL := baseURL + "/api/v1/meetings/" + created.Meeting.JoinCode

	var hostJoin joinResponse
	doJSON(t, host, http.MethodPost, meetingURL+"/join", nil, http.StatusCreated, &hostJoin)
	if hostJoin.Participant.Status != "JOINED" {
		t.Fatalf("host status = %s", hostJoin.Participant.Status)
	}
	var memberJoin joinResponse
	doJSON(t, member, http.MethodPost, meetingURL+"/join", nil, http.StatusCreated, &memberJoin)
	if memberJoin.Participant.Status != "WAITING_ROOM" {
		t.Fatalf("member status = %s", memberJoin.Participant.Status)
	}

	var participants struct {
		Participants []struct {
			ID     string `json:"id"`
			UserID string `json:"userId"`
			Status string `json:"status"`
		} `json:"participants"`
	}
	doJSON(t, host, http.MethodGet, meetingURL+"/participants", nil, http.StatusOK, &participants)
	waitingID := ""
	for _, participant := range participants.Participants {
		if participant.UserID == memberID && participant.Status == "WAITING_ROOM" {
			waitingID = participant.ID
		}
	}
	if waitingID == "" {
		t.Fatal("waiting participant was not visible to host")
	}
	doJSON(t, host, http.MethodPost, meetingURL+"/participants/"+waitingID+"/admit", nil, http.StatusOK, nil)

	var tokenResponse struct {
		Token    string `json:"token"`
		RoomName string `json:"roomName"`
	}
	doJSON(t, member, http.MethodPost, meetingURL+"/token", nil, http.StatusOK, &tokenResponse)
	if tokenResponse.Token == "" || tokenResponse.RoomName == "" {
		t.Fatal("realtime token was not issued after admission")
	}
	doJSON(t, member, http.MethodPost, meetingURL+"/leave", nil, http.StatusNoContent, nil)
	doJSON(t, host, http.MethodPost, meetingURL+"/moderation/end", nil, http.StatusOK, nil)

	var history struct {
		Meetings []struct {
			ID string `json:"id"`
		} `json:"meetings"`
	}
	doJSON(t, member, http.MethodGet, baseURL+"/api/v1/meetings/history", nil, http.StatusOK, &history)
	found := false
	for _, meeting := range history.Meetings {
		found = found || meeting.ID == created.Meeting.ID
	}
	if !found {
		t.Fatal("ended meeting missing from member history")
	}
	doJSON(t, host, http.MethodPost, baseURL+"/api/v1/auth/logout", nil, http.StatusNoContent, nil)
	doJSON(t, host, http.MethodGet, baseURL+"/api/v1/auth/me", nil, http.StatusUnauthorized, nil)

	var eventCount int
	err = database.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE tenant_id=$1 AND action IN ('auth.login','auth.login.failed','meeting.create','meeting.join','participant.admit','realtime.token.issue','participant.leave','meeting.end','auth.logout')`, tenantID).Scan(&eventCount)
	if err != nil || eventCount < 10 {
		t.Fatalf("security/lifecycle audit count = %d, error = %v", eventCount, err)
	}
	_ = hostID
}

type joinResponse struct {
	Participant struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"participant"`
}

func newAPIClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar, Timeout: 10 * time.Second}
}

func doJSON(t *testing.T, client *http.Client, method, endpoint string, input any, expectedStatus int, output any) {
	t.Helper()
	var body *bytes.Reader
	if input == nil {
		body = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		t.Fatal(err)
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		var failure any
		_ = json.NewDecoder(response.Body).Decode(&failure)
		t.Fatalf("%s %s returned %d, want %d: %#v", method, endpoint, response.StatusCode, expectedStatus, failure)
	}
	if output != nil {
		if err = json.NewDecoder(response.Body).Decode(output); err != nil {
			t.Fatalf("decode %s %s: %v", method, endpoint, err)
		}
	}
}

func cleanupTenant(t *testing.T, database *sql.DB, tenantID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	statements := []string{
		`DELETE FROM audit_events WHERE tenant_id=$1`,
		`DELETE FROM meetings WHERE tenant_id=$1`,
		`DELETE FROM groups WHERE tenant_id=$1`,
		`DELETE FROM users WHERE tenant_id=$1`,
		`DELETE FROM tenants WHERE id=$1`,
	}
	for _, statement := range statements {
		if _, err := database.ExecContext(ctx, statement, tenantID); err != nil {
			t.Errorf("integration cleanup failed: %v", err)
		}
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
