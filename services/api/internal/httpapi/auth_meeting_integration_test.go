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
	"net/url"
	"os"
	"strings"
	"sync/atomic"
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
	var tenantID, hostID, memberID, externalTenantID, externalUserID string
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
	if _, err = database.ExecContext(ctx, `INSERT INTO tenant_subscriptions(tenant_id,plan_key,status,current_period_started_at,current_period_ends_at) VALUES($1,'BUSINESS','ACTIVE',NOW(),NOW()+INTERVAL '1 day')`, tenantID); err != nil {
		t.Fatal(err)
	}
	err = database.QueryRowContext(ctx, `INSERT INTO users (tenant_id,email,username,display_name,password_hash,role,status) VALUES ($1,$2,'host','Integration Host',$3,'SUPER_ADMIN','ACTIVE') RETURNING id`, tenantID, "host-"+suffix+"@test.invalid", hash).Scan(&hostID)
	if err == nil {
		err = database.QueryRowContext(ctx, `INSERT INTO users (tenant_id,email,username,display_name,password_hash,role,status) VALUES ($1,$2,'member','Integration Member',$3,'MEMBER','ACTIVE') RETURNING id`, tenantID, "member-"+suffix+"@test.invalid", hash).Scan(&memberID)
	}
	if err != nil {
		t.Fatal(err)
	}
	externalTenantSlug := "external-" + suffix
	err = database.QueryRowContext(ctx, `INSERT INTO tenants (slug,name) VALUES ($1,'External Workspace') RETURNING id`, externalTenantSlug).Scan(&externalTenantID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupTenant(t, database, externalTenantID) })
	if _, err = database.ExecContext(ctx, `INSERT INTO tenant_subscriptions(tenant_id,plan_key,status,current_period_started_at,current_period_ends_at) VALUES($1,'BUSINESS','ACTIVE',NOW(),NOW()+INTERVAL '1 day')`, externalTenantID); err != nil {
		t.Fatal(err)
	}
	err = database.QueryRowContext(ctx, `INSERT INTO users (tenant_id,email,username,display_name,password_hash,role,status) VALUES ($1,$2,'external','External Visitor',$3,'TENANT_ADMIN','ACTIVE') RETURNING id`, externalTenantID, "external-"+suffix+"@test.invalid", hash).Scan(&externalUserID)
	if err != nil {
		t.Fatal(err)
	}

	host := newAPIClient(t)
	member := newAPIClient(t)
	external := newAPIClient(t)
	doJSON(t, host, http.MethodPost, baseURL+"/api/v1/auth/login", map[string]any{"tenant": tenantSlug, "identity": "host", "password": password}, http.StatusOK, nil)
	doJSON(t, member, http.MethodPost, baseURL+"/api/v1/auth/login", map[string]any{"tenant": tenantSlug, "identity": "member", "password": password}, http.StatusOK, nil)
	doJSON(t, external, http.MethodPost, baseURL+"/api/v1/auth/login", map[string]any{"tenant": externalTenantSlug, "identity": "external", "password": password}, http.StatusOK, nil)
	doJSON(t, member, http.MethodPost, baseURL+"/api/v1/auth/login", map[string]any{"tenant": tenantSlug, "identity": "member", "password": "wrong-password"}, http.StatusUnauthorized, nil)

	activePassword := "ActiveUser!Pass#2026"
	activeUsername := "active-" + suffix
	doJSON(t, host, http.MethodPost, baseURL+"/api/v1/admin/users", map[string]any{"email": activeUsername + "@test.invalid", "username": activeUsername, "displayName": "Active User", "role": "MEMBER", "status": "ACTIVE", "password": activePassword, "passwordConfirm": activePassword}, http.StatusCreated, nil)
	doJSON(t, host, http.MethodPost, baseURL+"/api/v1/admin/users", map[string]any{"email": activeUsername + "@test.invalid", "username": activeUsername, "displayName": "Duplicate User", "role": "MEMBER", "status": "ACTIVE", "password": activePassword, "passwordConfirm": activePassword}, http.StatusConflict, nil)
	activeClient := newAPIClient(t)
	doJSON(t, activeClient, http.MethodPost, baseURL+"/api/v1/auth/login", map[string]any{"tenant": tenantSlug, "identity": activeUsername, "password": activePassword}, http.StatusOK, nil)

	invitedUsername := "invited-" + suffix
	var invitation struct {
		InvitationPath string `json:"invitationPath"`
	}
	doJSON(t, host, http.MethodPost, baseURL+"/api/v1/admin/users", map[string]any{"email": invitedUsername + "@test.invalid", "username": invitedUsername, "displayName": "Invited User", "role": "MEMBER", "status": "INVITED", "password": "", "passwordConfirm": ""}, http.StatusCreated, &invitation)
	invitationURL, err := url.Parse(invitation.InvitationPath)
	if err != nil || invitationURL.Query().Get("token") == "" {
		t.Fatalf("invalid invitation path: %q", invitation.InvitationPath)
	}
	invitationToken := invitationURL.Query().Get("token")
	invitedPassword := "InvitedUser!Pass#2026"
	invitedClient := newAPIClient(t)
	doJSON(t, invitedClient, http.MethodPost, baseURL+"/api/v1/auth/invitations/accept", map[string]any{"token": invitationToken, "password": invitedPassword, "passwordConfirm": invitedPassword}, http.StatusOK, nil)
	doJSON(t, invitedClient, http.MethodGet, baseURL+"/api/v1/auth/me", nil, http.StatusOK, nil)
	doJSON(t, newAPIClient(t), http.MethodPost, baseURL+"/api/v1/auth/invitations/accept", map[string]any{"token": invitationToken, "password": invitedPassword, "passwordConfirm": invitedPassword}, http.StatusGone, nil)

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
	var externalPreview struct {
		Meeting struct {
			WorkspaceName string `json:"workspaceName"`
			ExternalGuest bool   `json:"externalGuest"`
		} `json:"meeting"`
	}
	doJSON(t, external, http.MethodGet, meetingURL, nil, http.StatusOK, &externalPreview)
	if !externalPreview.Meeting.ExternalGuest || externalPreview.Meeting.WorkspaceName != "Integration Test" {
		t.Fatalf("cross-workspace preview = %+v", externalPreview.Meeting)
	}

	var hostJoin joinResponse
	doJSON(t, host, http.MethodPost, meetingURL+"/join", recordingAcknowledgement(), http.StatusCreated, &hostJoin)
	if hostJoin.Participant.Status != "JOINED" {
		t.Fatalf("host status = %s", hostJoin.Participant.Status)
	}
	var memberJoin joinResponse
	doJSON(t, member, http.MethodPost, meetingURL+"/join", recordingAcknowledgement(), http.StatusCreated, &memberJoin)
	if memberJoin.Participant.Status != "WAITING_ROOM" {
		t.Fatalf("member status = %s", memberJoin.Participant.Status)
	}
	var refreshedMemberJoin joinResponse
	doJSON(t, member, http.MethodPost, meetingURL+"/join", recordingAcknowledgement(), http.StatusCreated, &refreshedMemberJoin)
	if refreshedMemberJoin.Participant.ID != memberJoin.Participant.ID {
		t.Fatalf("refresh created duplicate participant: first = %s, second = %s", memberJoin.Participant.ID, refreshedMemberJoin.Participant.ID)
	}
	if refreshedMemberJoin.Participant.Status != "WAITING_ROOM" {
		t.Fatalf("refresh changed waiting-room status = %s", refreshedMemberJoin.Participant.Status)
	}
	var externalJoin joinResponse
	doJSON(t, external, http.MethodPost, meetingURL+"/join", recordingAcknowledgement(), http.StatusCreated, &externalJoin)
	if externalJoin.Participant.Status != "WAITING_ROOM" {
		t.Fatalf("external participant status = %s", externalJoin.Participant.Status)
	}
	var refreshedExternalJoin joinResponse
	doJSON(t, external, http.MethodPost, meetingURL+"/join", recordingAcknowledgement(), http.StatusCreated, &refreshedExternalJoin)
	if refreshedExternalJoin.Participant.ID != externalJoin.Participant.ID {
		t.Fatalf("external refresh created duplicate participant: first = %s, second = %s", externalJoin.Participant.ID, refreshedExternalJoin.Participant.ID)
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
	externalWaitingID := ""
	for _, participant := range participants.Participants {
		if participant.UserID == memberID && participant.Status == "WAITING_ROOM" {
			waitingID = participant.ID
		}
		if participant.UserID == externalUserID && participant.Status == "WAITING_ROOM" {
			externalWaitingID = participant.ID
		}
	}
	if waitingID == "" {
		t.Fatal("waiting participant was not visible to host")
	}
	doJSON(t, host, http.MethodPost, meetingURL+"/participants/"+waitingID+"/admit", nil, http.StatusOK, nil)
	if externalWaitingID == "" {
		t.Fatal("external waiting participant was not visible to host")
	}
	doJSON(t, host, http.MethodPost, meetingURL+"/participants/"+externalWaitingID+"/admit", nil, http.StatusOK, nil)

	var tokenResponse struct {
		Token    string `json:"token"`
		RoomName string `json:"roomName"`
	}
	doJSON(t, member, http.MethodPost, meetingURL+"/token", nil, http.StatusOK, &tokenResponse)
	if tokenResponse.Token == "" || tokenResponse.RoomName == "" {
		t.Fatal("realtime token was not issued after admission")
	}
	if _, err = database.ExecContext(ctx, `UPDATE meeting_participants SET status='DISCONNECTED',left_at=NOW() WHERE id=$1`, memberJoin.Participant.ID); err != nil {
		t.Fatal(err)
	}
	var reconnectTokenResponse struct {
		Token string `json:"token"`
	}
	doJSON(t, member, http.MethodPost, meetingURL+"/token", nil, http.StatusOK, &reconnectTokenResponse)
	if reconnectTokenResponse.Token == "" {
		t.Fatal("realtime token was not reissued while participant was reconnecting")
	}
	var restoredStatus string
	if err = database.QueryRowContext(ctx, `SELECT status::text FROM meeting_participants WHERE id=$1`, memberJoin.Participant.ID).Scan(&restoredStatus); err != nil || restoredStatus != "JOINED" {
		t.Fatalf("participant reconnect status = %q, error = %v", restoredStatus, err)
	}
	var externalTokenResponse struct {
		Token    string `json:"token"`
		RoomName string `json:"roomName"`
	}
	doJSON(t, external, http.MethodPost, meetingURL+"/token", nil, http.StatusOK, &externalTokenResponse)
	if externalTokenResponse.Token == "" || externalTokenResponse.RoomName != tokenResponse.RoomName {
		t.Fatal("external realtime token was not issued for the host room")
	}
	doJSON(t, external, http.MethodPost, meetingURL+"/leave", nil, http.StatusNoContent, nil)
	doJSON(t, host, http.MethodPost, meetingURL+"/moderation/lock", nil, http.StatusOK, nil)
	doJSON(t, host, http.MethodPost, meetingURL+"/moderation/unlock", nil, http.StatusOK, nil)
	doJSON(t, host, http.MethodPost, meetingURL+"/participants/"+waitingID+"/promote", nil, http.StatusOK, nil)
	doJSON(t, host, http.MethodGet, meetingURL+"/participants", nil, http.StatusOK, &participants)
	promoted := false
	for _, participant := range participants.Participants {
		if participant.ID == waitingID {
			var role string
			err = database.QueryRowContext(ctx, `SELECT role FROM meeting_participants WHERE id=$1`, waitingID).Scan(&role)
			promoted = err == nil && role == "CO_HOST"
		}
	}
	if !promoted {
		t.Fatal("participant was not promoted to co-host")
	}
	doJSON(t, host, http.MethodPost, meetingURL+"/participants/"+waitingID+"/remove", nil, http.StatusOK, nil)
	doJSON(t, member, http.MethodPost, meetingURL+"/token", nil, http.StatusForbidden, nil)
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
	clientNumber := integrationClientSequence.Add(1)
	clientIP := fmt.Sprintf("198.51.100.%d", 1+(clientNumber%200))
	return &http.Client{Jar: jar, Timeout: 10 * time.Second, Transport: integrationForwardedTransport{clientIP: clientIP}}
}

var integrationClientSequence atomic.Uint32

type integrationForwardedTransport struct{ clientIP string }

func (transport integrationForwardedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("X-Forwarded-For", transport.clientIP)
	return http.DefaultTransport.RoundTrip(request)
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

func recordingAcknowledgement() map[string]any {
	return map[string]any{"recordingNoticeAcknowledged": true, "recordingConsentVersion": "2026-08-29"}
}

func cleanupTenant(t *testing.T, database *sql.DB, tenantID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	statements := []string{
		`DELETE FROM audit_events WHERE tenant_id=$1`,
		`DELETE FROM meetings WHERE tenant_id=$1`,
		`DELETE FROM groups WHERE tenant_id=$1`,
		`DELETE FROM user_invitations WHERE tenant_id=$1`,
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
