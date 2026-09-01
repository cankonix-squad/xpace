package httpapi

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
)

type governancePolicy struct {
	RecordingRetentionDays  int       `json:"recordingRetentionDays"`
	DriveTrashRetentionDays int       `json:"driveTrashRetentionDays"`
	ChatRetentionDays       int       `json:"chatRetentionDays"`
	AuditRetentionDays      int       `json:"auditRetentionDays"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

type legalHoldResourceResponse struct {
	Type       string    `json:"type"`
	ResourceID string    `json:"resourceId"`
	AddedAt    time.Time `json:"addedAt"`
}

type legalHoldResponse struct {
	ID         string                      `json:"id"`
	Name       string                      `json:"name"`
	Reason     string                      `json:"reason"`
	Status     string                      `json:"status"`
	CreatedAt  time.Time                   `json:"createdAt"`
	ReleasedAt *time.Time                  `json:"releasedAt,omitempty"`
	Resources  []legalHoldResourceResponse `json:"resources"`
}

type governanceRetentionResult struct {
	ChatMessages     int64 `json:"chatMessages"`
	ChatAttachments  int64 `json:"chatAttachments"`
	Recordings       int64 `json:"recordings"`
	RecordingObjects int64 `json:"recordingObjects"`
	DriveFiles       int64 `json:"driveFiles"`
	AuditEvents      int64 `json:"auditEvents"`
}

func (policy governancePolicy) validate() bool {
	return policy.RecordingRetentionDays >= 1 && policy.RecordingRetentionDays <= 3650 &&
		policy.DriveTrashRetentionDays >= 1 && policy.DriveTrashRetentionDays <= 3650 &&
		policy.ChatRetentionDays >= 1 && policy.ChatRetentionDays <= 3650 &&
		policy.AuditRetentionDays >= 30 && policy.AuditRetentionDays <= 3650
}

func validLegalHoldResourceType(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "RECORDING", "DRIVE_FILE", "CHAT_CONVERSATION":
		return true
	default:
		return false
	}
}

func (api *API) adminGovernancePolicy(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "governance.manage") {
		errorJSON(writer, http.StatusForbidden, "GOVERNANCE_REQUIRED", "governance administrator access is required")
		return
	}
	if request.Method == http.MethodGet {
		policy, err := api.loadGovernancePolicy(request, actor.TenantID)
		if err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load governance policy")
			return
		}
		respondJSON(writer, http.StatusOK, map[string]any{"policy": policy})
		return
	}
	var policy governancePolicy
	if err := decodeJSON(writer, request, &policy); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	if !policy.validate() {
		errorJSON(writer, http.StatusBadRequest, "INVALID_RETENTION_POLICY", "retention days must be 1–3650; audit retention must be at least 30 days")
		return
	}
	err := api.database.QueryRowContext(request.Context(), `INSERT INTO tenant_governance_policies(tenant_id,recording_retention_days,drive_trash_retention_days,chat_retention_days,audit_retention_days,updated_by) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(tenant_id) DO UPDATE SET recording_retention_days=EXCLUDED.recording_retention_days,drive_trash_retention_days=EXCLUDED.drive_trash_retention_days,chat_retention_days=EXCLUDED.chat_retention_days,audit_retention_days=EXCLUDED.audit_retention_days,updated_by=EXCLUDED.updated_by,updated_at=NOW() RETURNING updated_at`, actor.TenantID, policy.RecordingRetentionDays, policy.DriveTrashRetentionDays, policy.ChatRetentionDays, policy.AuditRetentionDays, actor.ID).Scan(&policy.UpdatedAt)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update governance policy")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "governance.policy.update", "tenant", actor.TenantID, map[string]any{"recordingDays": policy.RecordingRetentionDays, "driveTrashDays": policy.DriveTrashRetentionDays, "chatDays": policy.ChatRetentionDays, "auditDays": policy.AuditRetentionDays})
	respondJSON(writer, http.StatusOK, map[string]any{"policy": policy})
}

func (api *API) loadGovernancePolicy(request *http.Request, tenantID string) (governancePolicy, error) {
	return api.loadGovernancePolicyContext(request.Context(), tenantID)
}

func (api *API) loadGovernancePolicyContext(ctx context.Context, tenantID string) (governancePolicy, error) {
	var policy governancePolicy
	err := api.database.QueryRowContext(ctx, `INSERT INTO tenant_governance_policies(tenant_id) VALUES($1) ON CONFLICT(tenant_id) DO UPDATE SET tenant_id=EXCLUDED.tenant_id RETURNING recording_retention_days,drive_trash_retention_days,chat_retention_days,audit_retention_days,updated_at`, tenantID).Scan(&policy.RecordingRetentionDays, &policy.DriveTrashRetentionDays, &policy.ChatRetentionDays, &policy.AuditRetentionDays, &policy.UpdatedAt)
	return policy, err
}

func (api *API) adminLegalHolds(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "governance.manage") {
		errorJSON(writer, http.StatusForbidden, "GOVERNANCE_REQUIRED", "governance administrator access is required")
		return
	}
	if request.Method == http.MethodGet {
		api.listLegalHolds(writer, request, actor)
		return
	}
	var input struct{ Name, Reason string }
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	input.Name, input.Reason = strings.TrimSpace(input.Name), strings.TrimSpace(input.Reason)
	if len(input.Name) < 2 || len(input.Name) > 120 || len(input.Reason) < 2 || len(input.Reason) > 1000 {
		errorJSON(writer, http.StatusBadRequest, "INVALID_LEGAL_HOLD", "name must be 2–120 characters and reason 2–1000 characters")
		return
	}
	var id string
	err := api.database.QueryRowContext(request.Context(), `INSERT INTO legal_holds(tenant_id,name,reason,created_by) VALUES($1,$2,$3,$4) RETURNING id`, actor.TenantID, input.Name, input.Reason, actor.ID).Scan(&id)
	if err != nil {
		errorJSON(writer, http.StatusConflict, "LEGAL_HOLD_EXISTS", "an active or released hold with this name already exists")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "governance.hold.create", "legal_hold", id, map[string]any{"name": input.Name})
	respondJSON(writer, http.StatusCreated, map[string]any{"id": id, "status": "ACTIVE"})
}

func (api *API) listLegalHolds(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	rows, err := api.database.QueryContext(request.Context(), `SELECT h.id,h.name,h.reason,h.status,h.created_at,h.released_at,COALESCE(r.resource_type,''),COALESCE(r.resource_id::text,''),r.added_at FROM legal_holds h LEFT JOIN legal_hold_resources r ON r.hold_id=h.id AND r.tenant_id=h.tenant_id WHERE h.tenant_id=$1 ORDER BY h.created_at DESC,r.added_at`, actor.TenantID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load legal holds")
		return
	}
	defer rows.Close()
	items, positions := make([]legalHoldResponse, 0), map[string]int{}
	for rows.Next() {
		var hold legalHoldResponse
		var resourceType, resourceID string
		var addedAt sql.NullTime
		if err = rows.Scan(&hold.ID, &hold.Name, &hold.Reason, &hold.Status, &hold.CreatedAt, &hold.ReleasedAt, &resourceType, &resourceID, &addedAt); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load legal holds")
			return
		}
		position, exists := positions[hold.ID]
		if !exists {
			hold.Resources = make([]legalHoldResourceResponse, 0)
			items = append(items, hold)
			position = len(items) - 1
			positions[hold.ID] = position
		}
		if resourceID != "" && addedAt.Valid {
			items[position].Resources = append(items[position].Resources, legalHoldResourceResponse{Type: resourceType, ResourceID: resourceID, AddedAt: addedAt.Time})
		}
	}
	respondJSON(writer, http.StatusOK, map[string]any{"holds": items})
}

func (api *API) releaseLegalHold(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "governance.manage") {
		errorJSON(writer, http.StatusForbidden, "GOVERNANCE_REQUIRED", "governance administrator access is required")
		return
	}
	id := request.PathValue("holdID")
	result, err := api.database.ExecContext(request.Context(), `UPDATE legal_holds SET status='RELEASED',released_by=$1,released_at=NOW() WHERE id=$2 AND tenant_id=$3 AND status='ACTIVE'`, actor.ID, id, actor.TenantID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not release legal hold")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		errorJSON(writer, http.StatusNotFound, "LEGAL_HOLD_NOT_FOUND", "active legal hold not found")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "governance.hold.release", "legal_hold", id, nil)
	respondJSON(writer, http.StatusOK, map[string]string{"status": "RELEASED"})
}

func (api *API) legalHoldResource(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "governance.manage") {
		errorJSON(writer, http.StatusForbidden, "GOVERNANCE_REQUIRED", "governance administrator access is required")
		return
	}
	holdID, resourceType, resourceID := request.PathValue("holdID"), strings.ToUpper(strings.TrimSpace(request.PathValue("resourceType"))), request.PathValue("resourceID")
	if !validLegalHoldResourceType(resourceType) {
		errorJSON(writer, http.StatusBadRequest, "INVALID_RESOURCE_TYPE", "resourceType must be RECORDING, DRIVE_FILE, or CHAT_CONVERSATION")
		return
	}
	if request.Method == http.MethodPut {
		tx, err := api.database.BeginTx(request.Context(), &sql.TxOptions{})
		if err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not protect held resource")
			return
		}
		defer tx.Rollback()
		if _, err = tx.ExecContext(request.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, governanceResourceLockKey(actor.TenantID, resourceType, resourceID)); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not lock held resource")
			return
		}
		query := map[string]string{
			"RECORDING":         `SELECT EXISTS(SELECT 1 FROM recordings WHERE id=$1 AND tenant_id=$2 AND storage_deleted_at IS NULL)`,
			"DRIVE_FILE":        `SELECT EXISTS(SELECT 1 FROM drive_nodes WHERE id=$1 AND tenant_id=$2 AND kind='FILE')`,
			"CHAT_CONVERSATION": `SELECT EXISTS(SELECT 1 FROM chat_conversations WHERE id=$1 AND tenant_id=$2)`,
		}[resourceType]
		var exists bool
		if err := tx.QueryRowContext(request.Context(), query, resourceID, actor.TenantID).Scan(&exists); err != nil || !exists {
			errorJSON(writer, http.StatusNotFound, "RESOURCE_NOT_FOUND", "tenant resource not found")
			return
		}
		result, err := tx.ExecContext(request.Context(), `INSERT INTO legal_hold_resources(hold_id,tenant_id,resource_type,resource_id) SELECT id,tenant_id,$3,$4 FROM legal_holds WHERE id=$1 AND tenant_id=$2 AND status='ACTIVE' ON CONFLICT DO NOTHING`, holdID, actor.TenantID, resourceType, resourceID)
		if err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not add held resource")
			return
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			errorJSON(writer, http.StatusNotFound, "LEGAL_HOLD_NOT_FOUND", "active legal hold not found or resource already held")
			return
		}
		if err = tx.Commit(); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not commit held resource")
			return
		}
		_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "governance.hold.resource.add", resourceType, resourceID, map[string]any{"holdId": holdID})
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	result, err := api.database.ExecContext(request.Context(), `DELETE FROM legal_hold_resources r USING legal_holds h WHERE r.hold_id=$1 AND r.tenant_id=$2 AND r.resource_type=$3 AND r.resource_id=$4 AND h.id=r.hold_id AND h.tenant_id=r.tenant_id AND h.status='ACTIVE'`, holdID, actor.TenantID, resourceType, resourceID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not remove held resource")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		errorJSON(writer, http.StatusNotFound, "RESOURCE_NOT_FOUND", "held resource not found")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "governance.hold.resource.remove", resourceType, resourceID, map[string]any{"holdId": holdID})
	writer.WriteHeader(http.StatusNoContent)
}

func governanceResourceLockKey(tenantID, resourceType, resourceID string) string {
	return tenantID + ":" + resourceType + ":" + resourceID
}

func (api *API) runGovernanceRetention(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "governance.manage") {
		errorJSON(writer, http.StatusForbidden, "GOVERNANCE_REQUIRED", "governance administrator access is required")
		return
	}
	result, err := api.applyGovernanceRetention(request.Context(), actor.TenantID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "RETENTION_FAILED", err.Error())
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "governance.retention.run", "tenant", actor.TenantID, map[string]any{"chatMessages": result.ChatMessages, "chatAttachments": result.ChatAttachments, "recordings": result.Recordings, "recordingObjects": result.RecordingObjects, "driveFiles": result.DriveFiles, "auditEvents": result.AuditEvents})
	respondJSON(writer, http.StatusOK, map[string]any{"status": "completed", "affected": result})
}

func (api *API) applyGovernanceRetention(ctx context.Context, tenantID string) (governanceRetentionResult, error) {
	var result governanceRetentionResult
	policy, err := api.loadGovernancePolicyContext(ctx, tenantID)
	if err != nil {
		return result, fmt.Errorf("could not load governance policy")
	}
	result.ChatAttachments, err = api.purgeExpiredChatAttachments(ctx, tenantID, policy.ChatRetentionDays)
	if err != nil {
		return result, fmt.Errorf("chat attachment retention could not be applied")
	}
	chatResult, err := api.database.ExecContext(ctx, `UPDATE chat_messages m SET body='[removed by retention policy]',deleted_at=COALESCE(deleted_at,NOW()),edited_at=NOW() WHERE m.tenant_id=$1 AND m.created_at<NOW()-$2*INTERVAL '1 day' AND m.deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM legal_hold_resources r JOIN legal_holds h ON h.id=r.hold_id AND h.tenant_id=r.tenant_id WHERE r.tenant_id=m.tenant_id AND r.resource_type='CHAT_CONVERSATION' AND r.resource_id=m.conversation_id AND h.status='ACTIVE')`, tenantID, policy.ChatRetentionDays)
	if err != nil {
		return result, fmt.Errorf("chat retention could not be applied")
	}
	result.ChatMessages, _ = chatResult.RowsAffected()
	recordingResult, err := api.database.ExecContext(ctx, `UPDATE recordings r SET retention_expired_at=NOW(),updated_at=NOW() WHERE r.tenant_id=$1 AND r.created_at<NOW()-$2*INTERVAL '1 day' AND r.retention_expired_at IS NULL AND NOT EXISTS(SELECT 1 FROM legal_hold_resources hr JOIN legal_holds h ON h.id=hr.hold_id AND h.tenant_id=hr.tenant_id WHERE hr.tenant_id=r.tenant_id AND hr.resource_type='RECORDING' AND hr.resource_id=r.id AND h.status='ACTIVE')`, tenantID, policy.RecordingRetentionDays)
	if err != nil {
		return result, fmt.Errorf("recording retention could not be applied")
	}
	result.Recordings, _ = recordingResult.RowsAffected()
	result.RecordingObjects, err = api.purgeExpiredRecordingObjects(ctx, tenantID)
	if err != nil {
		return result, fmt.Errorf("recording object retention could not be applied")
	}
	result.DriveFiles = api.cleanupDriveRetention(ctx, tenantID)
	auditResult, err := api.database.ExecContext(ctx, `DELETE FROM audit_events WHERE tenant_id=$1 AND created_at<NOW()-$2*INTERVAL '1 day'`, tenantID, policy.AuditRetentionDays)
	if err != nil {
		return result, fmt.Errorf("audit retention could not be applied")
	}
	result.AuditEvents, _ = auditResult.RowsAffected()
	return result, nil
}

func (api *API) purgeExpiredChatAttachments(ctx context.Context, tenantID string, retentionDays int) (int64, error) {
	client, bucket, err := recordingObjectClient()
	if err != nil {
		return 0, err
	}
	rows, err := api.database.QueryContext(ctx, `SELECT a.id,a.object_key,a.conversation_id FROM chat_attachments a JOIN chat_messages m ON m.id=a.message_id AND m.tenant_id=a.tenant_id WHERE a.tenant_id=$1 AND m.created_at<NOW()-$2*INTERVAL '1 day' AND NOT EXISTS(SELECT 1 FROM legal_hold_resources r JOIN legal_holds h ON h.id=r.hold_id AND h.tenant_id=r.tenant_id WHERE r.tenant_id=a.tenant_id AND r.resource_type='CHAT_CONVERSATION' AND r.resource_id=a.conversation_id AND h.status='ACTIVE') ORDER BY a.created_at LIMIT 100`, tenantID, retentionDays)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type object struct{ id, key, conversationID string }
	items := make([]object, 0)
	for rows.Next() {
		var item object
		if err := rows.Scan(&item.id, &item.key, &item.conversationID); err != nil {
			return 0, err
		}
		items = append(items, item)
	}
	var removed int64
	for _, item := range items {
		tx, err := api.database.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			continue
		}
		if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, governanceResourceLockKey(tenantID, "CHAT_CONVERSATION", item.conversationID)); err != nil {
			tx.Rollback()
			continue
		}
		var held bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM chat_attachments a JOIN legal_hold_resources r ON r.tenant_id=a.tenant_id AND r.resource_type='CHAT_CONVERSATION' AND r.resource_id=a.conversation_id JOIN legal_holds h ON h.id=r.hold_id AND h.tenant_id=r.tenant_id AND h.status='ACTIVE' WHERE a.id=$1 AND a.tenant_id=$2)`, item.id, tenantID).Scan(&held); err != nil || held {
			tx.Rollback()
			continue
		}
		if err := client.RemoveObject(ctx, bucket, item.key, minio.RemoveObjectOptions{}); err != nil {
			tx.Rollback()
			continue
		}
		deleteResult, err := tx.ExecContext(ctx, `DELETE FROM chat_attachments a USING chat_messages m WHERE a.id=$1 AND a.tenant_id=$2 AND m.id=a.message_id AND m.tenant_id=a.tenant_id AND m.created_at<NOW()-$3*INTERVAL '1 day' AND NOT EXISTS(SELECT 1 FROM legal_hold_resources r JOIN legal_holds h ON h.id=r.hold_id AND h.tenant_id=r.tenant_id WHERE r.tenant_id=a.tenant_id AND r.resource_type='CHAT_CONVERSATION' AND r.resource_id=a.conversation_id AND h.status='ACTIVE')`, item.id, tenantID, retentionDays)
		if err == nil {
			count, _ := deleteResult.RowsAffected()
			if tx.Commit() == nil {
				removed += count
			}
		} else {
			tx.Rollback()
		}
	}
	return removed, rows.Err()
}

func (api *API) purgeExpiredRecordingObjects(ctx context.Context, tenantID string) (int64, error) {
	client, bucket, err := recordingObjectClient()
	if err != nil {
		return 0, err
	}
	rows, err := api.database.QueryContext(ctx, `SELECT r.id,r.object_key FROM recordings r WHERE r.tenant_id=$1 AND r.retention_expired_at IS NOT NULL AND r.storage_deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM legal_hold_resources hr JOIN legal_holds h ON h.id=hr.hold_id AND h.tenant_id=hr.tenant_id WHERE hr.tenant_id=r.tenant_id AND hr.resource_type='RECORDING' AND hr.resource_id=r.id AND h.status='ACTIVE') ORDER BY r.retention_expired_at LIMIT 50`, tenantID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type object struct{ id, key string }
	items := make([]object, 0)
	for rows.Next() {
		var item object
		if err := rows.Scan(&item.id, &item.key); err != nil {
			return 0, err
		}
		items = append(items, item)
	}
	var removed int64
	for _, item := range items {
		tx, err := api.database.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			continue
		}
		if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, governanceResourceLockKey(tenantID, "RECORDING", item.id)); err != nil {
			tx.Rollback()
			continue
		}
		var held bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM legal_hold_resources hr JOIN legal_holds h ON h.id=hr.hold_id AND h.tenant_id=hr.tenant_id WHERE hr.tenant_id=$1 AND hr.resource_type='RECORDING' AND hr.resource_id=$2 AND h.status='ACTIVE')`, tenantID, item.id).Scan(&held); err != nil || held {
			tx.Rollback()
			continue
		}
		if err := client.RemoveObject(ctx, bucket, item.key, minio.RemoveObjectOptions{}); err != nil {
			tx.Rollback()
			continue
		}
		updateResult, err := tx.ExecContext(ctx, `UPDATE recordings r SET storage_deleted_at=NOW(),size_bytes=0,updated_at=NOW() WHERE r.id=$1 AND r.tenant_id=$2 AND r.retention_expired_at IS NOT NULL AND r.storage_deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM legal_hold_resources hr JOIN legal_holds h ON h.id=hr.hold_id AND h.tenant_id=hr.tenant_id WHERE hr.tenant_id=r.tenant_id AND hr.resource_type='RECORDING' AND hr.resource_id=r.id AND h.status='ACTIVE')`, item.id, tenantID)
		if err == nil {
			count, _ := updateResult.RowsAffected()
			if tx.Commit() == nil {
				removed += count
			}
		} else {
			tx.Rollback()
		}
	}
	return removed, rows.Err()
}
