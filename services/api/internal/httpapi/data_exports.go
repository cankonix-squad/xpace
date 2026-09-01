package httpapi

import (
	"net/http"
	"strings"
	"time"
)

type dataExportResponse struct {
	ID              string     `json:"id"`
	ExportType      string     `json:"exportType"`
	Reason          string     `json:"reason"`
	Status          string     `json:"status"`
	RequestedByID   string     `json:"requestedById"`
	RequestedByName string     `json:"requestedByName"`
	ReviewedByName  string     `json:"reviewedByName,omitempty"`
	ReviewNote      string     `json:"reviewNote,omitempty"`
	SizeBytes       *int64     `json:"sizeBytes,omitempty"`
	SHA256          string     `json:"sha256,omitempty"`
	ExpiresAt       *time.Time `json:"expiresAt,omitempty"`
	DownloadedAt    *time.Time `json:"downloadedAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	CanReview       bool       `json:"canReview"`
}

func validDataExportType(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "FULL", "AUDIT", "DIRECTORY":
		return true
	default:
		return false
	}
}

func (api *API) adminDataExports(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "governance.manage") {
		errorJSON(writer, http.StatusForbidden, "GOVERNANCE_REQUIRED", "governance administrator access is required")
		return
	}
	if request.Method == http.MethodGet {
		rows, err := api.database.QueryContext(request.Context(), `SELECT e.id,e.export_type,e.reason,e.status,e.requested_by,requester.display_name,COALESCE(reviewer.display_name,''),e.review_note,e.size_bytes,COALESCE(e.sha256,''),e.expires_at,e.downloaded_at,e.created_at,e.updated_at FROM data_export_requests e JOIN users requester ON requester.id=e.requested_by AND requester.tenant_id=e.tenant_id LEFT JOIN users reviewer ON reviewer.id=e.reviewed_by AND reviewer.tenant_id=e.tenant_id WHERE e.tenant_id=$1 ORDER BY e.created_at DESC LIMIT 100`, actor.TenantID)
		if err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load data exports")
			return
		}
		defer rows.Close()
		items := make([]dataExportResponse, 0)
		for rows.Next() {
			var item dataExportResponse
			if err := rows.Scan(&item.ID, &item.ExportType, &item.Reason, &item.Status, &item.RequestedByID, &item.RequestedByName, &item.ReviewedByName, &item.ReviewNote, &item.SizeBytes, &item.SHA256, &item.ExpiresAt, &item.DownloadedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
				errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load data exports")
				return
			}
			item.CanReview = item.Status == "PENDING" && item.RequestedByID != actor.ID
			items = append(items, item)
		}
		respondJSON(writer, http.StatusOK, map[string]any{"exports": items})
		return
	}
	var input struct {
		ExportType string `json:"exportType"`
		Reason     string `json:"reason"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	input.ExportType, input.Reason = strings.ToUpper(strings.TrimSpace(input.ExportType)), strings.TrimSpace(input.Reason)
	if !validDataExportType(input.ExportType) || len(input.Reason) < 5 || len(input.Reason) > 1000 {
		errorJSON(writer, http.StatusBadRequest, "INVALID_EXPORT_REQUEST", "exportType must be FULL, AUDIT, or DIRECTORY and reason must be 5–1000 characters")
		return
	}
	var id string
	err := api.database.QueryRowContext(request.Context(), `INSERT INTO data_export_requests(tenant_id,requested_by,export_type,reason) VALUES($1,$2,$3,$4) RETURNING id`, actor.TenantID, actor.ID, input.ExportType, input.Reason).Scan(&id)
	if err != nil {
		errorJSON(writer, http.StatusConflict, "EXPORT_ALREADY_PENDING", "you already have an export awaiting review or processing")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "governance.export.request", "data_export", id, map[string]any{"exportType": input.ExportType, "reason": input.Reason})
	respondJSON(writer, http.StatusCreated, map[string]any{"id": id, "status": "PENDING"})
}

func (api *API) reviewDataExport(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "governance.manage") {
		errorJSON(writer, http.StatusForbidden, "GOVERNANCE_REQUIRED", "governance administrator access is required")
		return
	}
	var input struct {
		Note string `json:"note"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	input.Note = strings.TrimSpace(input.Note)
	action := "approve"
	status := "APPROVED"
	if strings.HasSuffix(request.URL.Path, "/reject") {
		action, status = "reject", "REJECTED"
		if len(input.Note) < 5 {
			errorJSON(writer, http.StatusBadRequest, "REJECTION_REASON_REQUIRED", "a rejection note of at least 5 characters is required")
			return
		}
	}
	if len(input.Note) > 1000 {
		errorJSON(writer, http.StatusBadRequest, "INVALID_REVIEW_NOTE", "review note must be 1000 characters or fewer")
		return
	}
	id := request.PathValue("exportID")
	result, err := api.database.ExecContext(request.Context(), `UPDATE data_export_requests SET status=$1,reviewed_by=$2,review_note=$3,reviewed_at=NOW(),updated_at=NOW() WHERE id=$4 AND tenant_id=$5 AND status='PENDING' AND requested_by<>$2`, status, actor.ID, input.Note, id, actor.TenantID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not review data export")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		errorJSON(writer, http.StatusConflict, "FOUR_EYES_REQUIRED", "a pending export must be reviewed by a different administrator")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "governance.export."+action, "data_export", id, map[string]any{"note": input.Note})
	respondJSON(writer, http.StatusOK, map[string]string{"status": status})
}

func (api *API) downloadDataExport(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "governance.manage") {
		errorJSON(writer, http.StatusForbidden, "GOVERNANCE_REQUIRED", "governance administrator access is required")
		return
	}
	id := request.PathValue("exportID")
	var objectKey string
	var expiresAt time.Time
	err := api.database.QueryRowContext(request.Context(), `SELECT object_key,expires_at FROM data_export_requests WHERE id=$1 AND tenant_id=$2 AND status='READY' AND expires_at>NOW()`, id, actor.TenantID).Scan(&objectKey, &expiresAt)
	if err != nil {
		errorJSON(writer, http.StatusNotFound, "EXPORT_NOT_AVAILABLE", "export is not ready or has expired")
		return
	}
	client, bucket, err := recordingObjectClient()
	if err != nil {
		errorJSON(writer, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "export storage is unavailable")
		return
	}
	url, err := client.PresignedGetObject(request.Context(), bucket, objectKey, 5*time.Minute, nil)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not issue export download")
		return
	}
	_, _ = api.database.ExecContext(request.Context(), `UPDATE data_export_requests SET downloaded_at=NOW(),updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, id, actor.TenantID)
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "governance.export.download", "data_export", id, nil)
	respondJSON(writer, http.StatusOK, map[string]any{"url": url.String(), "expiresAt": time.Now().Add(5 * time.Minute).UTC(), "archiveExpiresAt": expiresAt})
}
