//go:build integration

package httpapi_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	passwordauth "github.com/cankonix/xpace/api/internal/auth"
)

// TestIntegrationTenantIsolation exercises authenticated object references from
// one tenant against resources owned by another tenant. Meeting preview/join is
// the sole intentional cross-workspace exception and is tested separately from
// management, moderation, analytics, and recording access.
func TestIntegrationTenantIsolation(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err = database.PingContext(ctx); err != nil {
		t.Fatalf("postgres unavailable: %v", err)
	}
	cleanupStaleIsolationTenants(t, database)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	password := "Isolation!Pass#2026"
	hash, err := passwordauth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	victimTenantID, victimSlug := createIsolationTenant(t, ctx, database, "victim-"+suffix, "Victim Workspace")
	attackerTenantID, attackerSlug := createIsolationTenant(t, ctx, database, "attacker-"+suffix, "Attacker Workspace")
	t.Cleanup(func() { cleanupIsolationTenant(t, database, attackerTenantID) })
	t.Cleanup(func() { cleanupIsolationTenant(t, database, victimTenantID) })

	victimAdminID := createIsolationUser(t, ctx, database, victimTenantID, "victim-admin", "Victim Admin", "TENANT_ADMIN", suffix, hash)
	victimMemberID := createIsolationUser(t, ctx, database, victimTenantID, "victim-member", "Victim Member", "MEMBER", suffix, hash)
	attackerAdminID := createIsolationUser(t, ctx, database, attackerTenantID, "attacker-admin", "Attacker Admin", "TENANT_ADMIN", suffix, hash)
	attackerMemberID := createIsolationUser(t, ctx, database, attackerTenantID, "attacker-member", "Attacker Member", "MEMBER", suffix, hash)
	attackerGuestID := createIsolationUser(t, ctx, database, attackerTenantID, "attacker-guest", "Attacker Guest", "GUEST", suffix, hash)

	victim := newAPIClient(t)
	attacker := newAPIClient(t)
	member := newAPIClient(t)
	guest := newAPIClient(t)
	doJSON(t, victim, http.MethodPost, baseURL+"/api/v1/auth/login", map[string]any{"tenant": victimSlug, "identity": "victim-admin", "password": password}, http.StatusOK, nil)
	doJSON(t, attacker, http.MethodPost, baseURL+"/api/v1/auth/login", map[string]any{"tenant": attackerSlug, "identity": "attacker-admin", "password": password}, http.StatusOK, nil)
	doJSON(t, member, http.MethodPost, baseURL+"/api/v1/auth/login", map[string]any{"tenant": attackerSlug, "identity": "attacker-member", "password": password}, http.StatusOK, nil)
	doJSON(t, guest, http.MethodPost, baseURL+"/api/v1/auth/login", map[string]any{"tenant": attackerSlug, "identity": "attacker-guest", "password": password}, http.StatusOK, nil)

	var group struct {
		Group struct {
			ID string `json:"id"`
		} `json:"group"`
	}
	doJSON(t, victim, http.MethodPost, baseURL+"/api/v1/admin/groups", map[string]any{"name": "Victim Group", "description": "tenant isolation target"}, http.StatusCreated, &group)

	var room struct {
		Room struct {
			ID string `json:"id"`
		} `json:"room"`
	}
	doJSON(t, victim, http.MethodPost, baseURL+"/api/v1/rooms", map[string]any{"name": "Victim Room", "description": "private", "visibility": "PRIVATE", "memberIds": []string{victimMemberID}}, http.StatusCreated, &room)

	var node struct {
		Node struct {
			ID string `json:"id"`
		} `json:"node"`
	}
	doJSON(t, victim, http.MethodPost, baseURL+"/api/v1/drive/folders", map[string]any{"name": "Victim Folder"}, http.StatusCreated, &node)

	var event struct {
		Event struct {
			ID string `json:"id"`
		} `json:"event"`
	}
	startsAt := time.Now().UTC().Add(time.Hour)
	doJSON(t, victim, http.MethodPost, baseURL+"/api/v1/calendar/events", map[string]any{"title": "Victim Event", "description": "private", "timezone": "Asia/Jakarta", "startsAt": startsAt, "endsAt": startsAt.Add(time.Hour), "attendeeIds": []string{victimMemberID}}, http.StatusCreated, &event)

	var conversation struct {
		Conversation struct {
			ID string `json:"id"`
		} `json:"conversation"`
	}
	doJSON(t, victim, http.MethodPost, baseURL+"/api/v1/chat/conversations", map[string]any{"type": "DIRECT", "memberIds": []string{victimMemberID}}, http.StatusCreated, &conversation)

	var meeting struct {
		Meeting struct {
			ID       string `json:"id"`
			JoinCode string `json:"joinCode"`
		} `json:"meeting"`
	}
	doJSON(t, victim, http.MethodPost, baseURL+"/api/v1/meetings", map[string]any{"title": "Victim Meeting"}, http.StatusCreated, &meeting)

	var hold struct {
		ID string `json:"id"`
	}
	doJSON(t, victim, http.MethodPost, baseURL+"/api/v1/admin/governance/holds", map[string]any{"name": "Victim Hold", "reason": "tenant isolation verification"}, http.StatusCreated, &hold)

	var customRole struct {
		Role struct {
			ID string `json:"id"`
		} `json:"role"`
	}
	doJSON(t, victim, http.MethodPost, baseURL+"/api/v1/admin/roles", map[string]any{"name": "Victim Auditor", "description": "private role", "permissions": []string{"audit.read"}}, http.StatusCreated, &customRole)

	var incident struct {
		ID string `json:"id"`
	}
	doJSON(t, victim, http.MethodPost, baseURL+"/api/v1/admin/incidents", map[string]any{"title": "Victim Incident", "description": "private incident", "severity": "P3", "source": "MANUAL"}, http.StatusCreated, &incident)

	var exportRequest struct {
		ID string `json:"id"`
	}
	doJSON(t, victim, http.MethodPost, baseURL+"/api/v1/admin/governance/exports", map[string]any{"exportType": "DIRECTORY", "reason": "tenant isolation verification"}, http.StatusCreated, &exportRequest)

	var recordingID, notificationID, victimSessionID string
	if err = database.QueryRowContext(ctx, `INSERT INTO recordings(tenant_id,meeting_id,started_by,object_key,status,stopped_at) VALUES($1,$2,$3,$4,'READY',NOW()) RETURNING id`, victimTenantID, meeting.Meeting.ID, victimAdminID, "pentest/"+suffix+".mp4").Scan(&recordingID); err != nil {
		t.Fatal(err)
	}
	if err = database.QueryRowContext(ctx, `INSERT INTO notifications(tenant_id,recipient_id,actor_id,type,conversation_id,payload) VALUES($1,$2,$3,'CHAT_MENTION',$4,'{}') RETURNING id`, victimTenantID, victimAdminID, victimMemberID, conversation.Conversation.ID).Scan(&notificationID); err != nil {
		t.Fatal(err)
	}
	if err = database.QueryRowContext(ctx, `SELECT id FROM sessions WHERE user_id=$1 AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1`, victimAdminID).Scan(&victimSessionID); err != nil {
		t.Fatal(err)
	}

	t.Run("member and guest cannot enter admin or platform", func(t *testing.T) {
		doJSON(t, member, http.MethodGet, baseURL+"/api/v1/admin/users", nil, http.StatusForbidden, nil)
		doJSON(t, member, http.MethodGet, baseURL+"/api/v1/admin/governance/policy", nil, http.StatusForbidden, nil)
		doJSON(t, guest, http.MethodGet, baseURL+"/api/v1/admin/users", nil, http.StatusForbidden, nil)
		doJSON(t, guest, http.MethodGet, baseURL+"/api/v1/platform/overview", nil, http.StatusForbidden, nil)
	})

	t.Run("foreign directory and user IDs are hidden", func(t *testing.T) {
		doJSON(t, attacker, http.MethodGet, baseURL+"/api/v1/directory/users/"+victimMemberID+"/avatar", nil, http.StatusNotFound, nil)
		doJSON(t, attacker, http.MethodPatch, baseURL+"/api/v1/admin/users/"+victimMemberID, map[string]any{"role": "MEMBER", "status": "DEACTIVATED"}, http.StatusNotFound, nil)
		assertUserStillActive(t, ctx, database, victimTenantID, victimMemberID)
	})

	t.Run("foreign collaboration resources are inaccessible", func(t *testing.T) {
		doJSON(t, attacker, http.MethodGet, baseURL+"/api/v1/rooms/"+room.Room.ID, nil, http.StatusNotFound, nil)
		doJSON(t, attacker, http.MethodPut, baseURL+"/api/v1/rooms/"+room.Room.ID+"/members/"+attackerMemberID, map[string]any{"role": "MEMBER"}, http.StatusForbidden, nil)
		doJSON(t, attacker, http.MethodPatch, baseURL+"/api/v1/drive/nodes/"+node.Node.ID, map[string]any{"name": "stolen"}, http.StatusForbidden, nil)
		doJSON(t, attacker, http.MethodPatch, baseURL+"/api/v1/calendar/events/"+event.Event.ID+"/response", map[string]any{"status": "ACCEPTED"}, http.StatusNotFound, nil)
		doJSON(t, attacker, http.MethodGet, baseURL+"/api/v1/chat/conversations/"+conversation.Conversation.ID+"/messages", nil, http.StatusNotFound, nil)
	})

	t.Run("cross-workspace meeting exposes preview only", func(t *testing.T) {
		doJSON(t, attacker, http.MethodGet, baseURL+"/api/v1/meetings/"+meeting.Meeting.JoinCode, nil, http.StatusOK, nil)
		doJSON(t, attacker, http.MethodDelete, baseURL+"/api/v1/meetings/"+meeting.Meeting.JoinCode, nil, http.StatusNotFound, nil)
		doJSON(t, attacker, http.MethodPost, baseURL+"/api/v1/meetings/"+meeting.Meeting.JoinCode+"/moderation/lock", nil, http.StatusNotFound, nil)
		doJSON(t, attacker, http.MethodGet, baseURL+"/api/v1/meetings/"+meeting.Meeting.JoinCode+"/participants", nil, http.StatusForbidden, nil)
		doJSON(t, attacker, http.MethodGet, baseURL+"/api/v1/admin/meetings/"+meeting.Meeting.ID, nil, http.StatusNotFound, nil)
		doJSON(t, attacker, http.MethodGet, baseURL+"/api/v1/recordings/"+recordingID+"/file", nil, http.StatusNotFound, nil)
	})

	t.Run("foreign governance and security objects are immutable", func(t *testing.T) {
		doJSON(t, attacker, http.MethodPatch, baseURL+"/api/v1/admin/groups/"+group.Group.ID, map[string]any{"name": "stolen", "description": "stolen"}, http.StatusNotFound, nil)
		doJSON(t, attacker, http.MethodPut, baseURL+"/api/v1/admin/roles/"+customRole.Role.ID, map[string]any{"name": "stolen", "description": "stolen", "permissions": []string{"audit.read"}}, http.StatusNotFound, nil)
		doJSON(t, attacker, http.MethodGet, baseURL+"/api/v1/admin/incidents/"+incident.ID, nil, http.StatusNotFound, nil)
		doJSON(t, attacker, http.MethodPost, baseURL+"/api/v1/admin/governance/holds/"+hold.ID+"/release", nil, http.StatusNotFound, nil)
		doJSON(t, attacker, http.MethodPost, baseURL+"/api/v1/admin/governance/exports/"+exportRequest.ID+"/approve", map[string]any{"note": "attacker approval"}, http.StatusConflict, nil)
		doJSON(t, attacker, http.MethodDelete, baseURL+"/api/v1/security/sessions/"+victimSessionID, nil, http.StatusNotFound, nil)
		doJSON(t, attacker, http.MethodPost, baseURL+"/api/v1/notifications/"+notificationID+"/read", nil, http.StatusNoContent, nil)
		doJSON(t, victim, http.MethodGet, baseURL+"/api/v1/auth/me", nil, http.StatusOK, nil)
		assertNotificationUnread(t, ctx, database, victimTenantID, notificationID)
	})

	t.Run("tenant admin is not platform admin", func(t *testing.T) {
		doJSON(t, attacker, http.MethodGet, baseURL+"/api/v1/platform/overview", nil, http.StatusForbidden, nil)
		doJSON(t, attacker, http.MethodGet, baseURL+"/api/v1/platform/tenants/"+victimTenantID, nil, http.StatusForbidden, nil)
	})

	_ = attackerAdminID
	_ = attackerGuestID
}

func createIsolationTenant(t *testing.T, ctx context.Context, database *sql.DB, slug, name string) (string, string) {
	t.Helper()
	var id string
	if err := database.QueryRowContext(ctx, `INSERT INTO tenants(slug,name) VALUES($1,$2) RETURNING id`, slug, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO tenant_subscriptions(tenant_id,plan_key,status,current_period_started_at,current_period_ends_at) VALUES($1,'BUSINESS','ACTIVE',NOW(),NOW()+INTERVAL '1 day')`, id); err != nil {
		t.Fatal(err)
	}
	return id, slug
}

func createIsolationUser(t *testing.T, ctx context.Context, database *sql.DB, tenantID, username, displayName, role, suffix, hash string) string {
	t.Helper()
	var id string
	email := username + "-" + suffix + "@test.invalid"
	if err := database.QueryRowContext(ctx, `INSERT INTO users(tenant_id,email,username,display_name,password_hash,role,status) VALUES($1,$2,$3,$4,$5,$6,'ACTIVE') RETURNING id`, tenantID, email, username, displayName, hash, role).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func cleanupIsolationTenant(t *testing.T, database *sql.DB, tenantID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	statements := []string{
		`DELETE FROM meetings WHERE tenant_id=$1`,
		`DELETE FROM workspace_rooms WHERE tenant_id=$1`,
		`DELETE FROM calendar_events WHERE tenant_id=$1`,
		`DELETE FROM drive_nodes WHERE tenant_id=$1`,
		`DELETE FROM chat_conversations WHERE tenant_id=$1`,
		`DELETE FROM data_export_requests WHERE tenant_id=$1`,
		`DELETE FROM legal_holds WHERE tenant_id=$1`,
		`DELETE FROM incidents WHERE tenant_id=$1`,
		`DELETE FROM custom_roles WHERE tenant_id=$1`,
		`DELETE FROM groups WHERE tenant_id=$1`,
		`DELETE FROM notifications WHERE tenant_id=$1`,
		`DELETE FROM audit_events WHERE tenant_id=$1`,
		`DELETE FROM platform_support_access WHERE tenant_id=$1`,
		`DELETE FROM user_invitations WHERE tenant_id=$1`,
		`DELETE FROM users WHERE tenant_id=$1`,
		`DELETE FROM tenants WHERE id=$1`,
	}
	for _, statement := range statements {
		if _, err := database.ExecContext(ctx, statement, tenantID); err != nil {
			t.Errorf("isolation cleanup failed: %v", err)
			return
		}
	}
}

func cleanupStaleIsolationTenants(t *testing.T, database *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rows, err := database.QueryContext(ctx, `SELECT id FROM tenants WHERE slug ~ '^(victim|attacker)-[0-9]+$'`)
	if err != nil {
		t.Fatal(err)
	}
	var tenantIDs []string
	for rows.Next() {
		var tenantID string
		if rows.Scan(&tenantID) == nil {
			tenantIDs = append(tenantIDs, tenantID)
		}
	}
	rows.Close()
	for _, tenantID := range tenantIDs {
		cleanupIsolationTenant(t, database, tenantID)
	}
}

func assertUserStillActive(t *testing.T, ctx context.Context, database *sql.DB, tenantID, userID string) {
	t.Helper()
	var status string
	if err := database.QueryRowContext(ctx, `SELECT status FROM users WHERE tenant_id=$1 AND id=$2`, tenantID, userID).Scan(&status); err != nil || status != "ACTIVE" {
		t.Fatalf("foreign user was mutated: status=%q err=%v", status, err)
	}
}

func assertNotificationUnread(t *testing.T, ctx context.Context, database *sql.DB, tenantID, notificationID string) {
	t.Helper()
	var unread bool
	if err := database.QueryRowContext(ctx, `SELECT read_at IS NULL FROM notifications WHERE tenant_id=$1 AND id=$2`, tenantID, notificationID).Scan(&unread); err != nil || !unread {
		t.Fatalf("foreign notification was mutated: unread=%v err=%v", unread, err)
	}
}
