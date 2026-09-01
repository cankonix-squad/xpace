package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
)

const maxDriveUpload = 100 << 20

type driveNode struct {
	ID             string     `json:"id"`
	ParentID       *string    `json:"parentId,omitempty"`
	Kind           string     `json:"kind"`
	Name           string     `json:"name"`
	ContentType    string     `json:"contentType,omitempty"`
	SizeBytes      int64      `json:"sizeBytes"`
	Version        int        `json:"version"`
	Permission     string     `json:"permission"`
	RetentionUntil *time.Time `json:"retentionUntil,omitempty"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type driveVersion struct {
	Version     int       `json:"version"`
	ContentType string    `json:"contentType"`
	SizeBytes   int64     `json:"sizeBytes"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (api *API) driveNodes(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	switch request.Method {
	case http.MethodGet:
		api.cleanupDriveRetention(request.Context(), actor.TenantID)
		parent := strings.TrimSpace(request.URL.Query().Get("parentId"))
		roomID := strings.TrimSpace(request.URL.Query().Get("roomId"))
		if roomID != "" && !api.isRoomMember(request, actor, roomID) {
			errorJSON(writer, 404, "NOT_FOUND", "room not found")
			return
		}
		rows, err := api.database.QueryContext(request.Context(), `SELECT n.id,n.parent_id,n.kind,n.name,COALESCE(n.content_type,''),n.size_bytes,n.version,CASE WHEN n.owner_id=$2 THEN 'OWNER' ELSE COALESCE(s.permission,'') END,n.retention_until,n.updated_at FROM drive_nodes n LEFT JOIN drive_shares s ON s.node_id=n.id AND s.tenant_id=n.tenant_id AND s.user_id=$2 WHERE n.tenant_id=$1 AND n.deleted_at IS NULL AND (($3='' AND n.parent_id IS NULL) OR n.parent_id=NULLIF($3,'')::uuid) AND (($4='' AND n.room_id IS NULL) OR n.room_id=NULLIF($4,'')::uuid) AND (n.owner_id=$2 OR s.user_id=$2) ORDER BY n.kind DESC,n.name`, actor.TenantID, actor.ID, parent, roomID)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load drive")
			return
		}
		defer rows.Close()
		items := make([]driveNode, 0)
		for rows.Next() {
			var item driveNode
			if err := rows.Scan(&item.ID, &item.ParentID, &item.Kind, &item.Name, &item.ContentType, &item.SizeBytes, &item.Version, &item.Permission, &item.RetentionUntil, &item.UpdatedAt); err != nil {
				errorJSON(writer, 500, "INTERNAL_ERROR", "could not load drive")
				return
			}
			items = append(items, item)
		}
		var used int64
		_ = api.database.QueryRowContext(request.Context(), `SELECT COALESCE(SUM(size_bytes),0) FROM drive_nodes WHERE tenant_id=$1 AND deleted_at IS NULL AND kind='FILE'`, actor.TenantID).Scan(&used)
		respondJSON(writer, 200, map[string]any{"nodes": items, "quota": map[string]any{"usedBytes": used, "limitBytes": int64(10 << 30)}})
	case http.MethodPost:
		var input struct {
			Name     string `json:"name"`
			ParentID string `json:"parentId"`
			RoomID   string `json:"roomId"`
		}
		if err := decodeJSON(writer, request, &input); err != nil {
			errorJSON(writer, 400, "INVALID_INPUT", err.Error())
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Name == "" || len(input.Name) > 255 {
			errorJSON(writer, 400, "INVALID_INPUT", "folder name is invalid")
			return
		}
		if input.ParentID != "" && !api.canEditDriveNode(request, actor, input.ParentID) {
			errorJSON(writer, 403, "FORBIDDEN", "parent folder is unavailable")
			return
		}
		if input.RoomID != "" && !api.isRoomMember(request, actor, input.RoomID) {
			errorJSON(writer, 404, "NOT_FOUND", "room not found")
			return
		}
		var item driveNode
		err := api.database.QueryRowContext(request.Context(), `INSERT INTO drive_nodes(tenant_id,parent_id,room_id,owner_id,kind,name) VALUES($1,NULLIF($2,'')::uuid,NULLIF($3,'')::uuid,$4,'FOLDER',$5) RETURNING id,parent_id,kind,name,size_bytes,version,updated_at`, actor.TenantID, input.ParentID, input.RoomID, actor.ID, input.Name).Scan(&item.ID, &item.ParentID, &item.Kind, &item.Name, &item.SizeBytes, &item.Version, &item.UpdatedAt)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not create folder")
			return
		}
		item.Permission = "OWNER"
		_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "drive.folder.created", "drive_node", item.ID, nil)
		respondJSON(writer, 201, map[string]any{"node": item})
	}
}

func (api *API) driveUpload(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if request.ContentLength > maxDriveUpload+1<<20 {
		errorJSON(writer, 413, "FILE_TOO_LARGE", "file must be 100 MB or smaller")
		return
	}
	if err := request.ParseMultipartForm(maxDriveUpload); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", "invalid upload")
		return
	}
	file, header, err := request.FormFile("file")
	if err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", "file is required")
		return
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > maxDriveUpload {
		errorJSON(writer, 413, "FILE_TOO_LARGE", "file must be 100 MB or smaller")
		return
	}
	parent := strings.TrimSpace(request.FormValue("parentId"))
	roomID := strings.TrimSpace(request.FormValue("roomId"))
	if roomID != "" && !api.isRoomMember(request, actor, roomID) {
		errorJSON(writer, 404, "NOT_FOUND", "room not found")
		return
	}
	if parent != "" && !api.canEditDriveNode(request, actor, parent) {
		errorJSON(writer, 403, "FORBIDDEN", "parent folder is unavailable")
		return
	}
	if err = api.enforceTenantQuota(request.Context(), actor.TenantID, "drive", header.Size); err != nil {
		if !respondEntitlementError(writer, err) {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not verify workspace quota")
		}
		return
	}
	name := filepath.Base(strings.TrimSpace(header.Filename))
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	objectKey := fmt.Sprintf("drive/%s/%d-%s", actor.TenantID, time.Now().UnixNano(), name)
	client, bucket, err := recordingObjectClient()
	if err != nil {
		errorJSON(writer, 503, "STORAGE_UNAVAILABLE", "drive storage is unavailable")
		return
	}
	if _, err = client.PutObject(request.Context(), bucket, objectKey, file, header.Size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		errorJSON(writer, 502, "STORAGE_ERROR", "could not store file")
		return
	}
	var item driveNode
	retention := time.Now().Add(30 * 24 * time.Hour)
	err = api.database.QueryRowContext(request.Context(), `INSERT INTO drive_nodes(tenant_id,parent_id,room_id,owner_id,kind,name,object_key,content_type,size_bytes,retention_until) VALUES($1,NULLIF($2,'')::uuid,NULLIF($3,'')::uuid,$4,'FILE',$5,$6,$7,$8,$9) RETURNING id,parent_id,kind,name,content_type,size_bytes,version,retention_until,updated_at`, actor.TenantID, parent, roomID, actor.ID, name, objectKey, contentType, header.Size, retention).Scan(&item.ID, &item.ParentID, &item.Kind, &item.Name, &item.ContentType, &item.SizeBytes, &item.Version, &item.RetentionUntil, &item.UpdatedAt)
	if err != nil {
		_ = client.RemoveObject(request.Context(), bucket, objectKey, minio.RemoveObjectOptions{})
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not save file metadata")
		return
	}
	item.Permission = "OWNER"
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "drive.file.uploaded", "drive_node", item.ID, map[string]any{"sizeBytes": item.SizeBytes})
	respondJSON(writer, 201, map[string]any{"node": item})
}

func (api *API) driveDownload(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	id := request.PathValue("nodeID")
	var key, name string
	if err := api.database.QueryRowContext(request.Context(), `SELECT n.object_key,n.name FROM drive_nodes n LEFT JOIN drive_shares s ON s.node_id=n.id AND s.tenant_id=n.tenant_id AND s.user_id=$3 WHERE n.id=$1 AND n.tenant_id=$2 AND n.kind='FILE' AND n.deleted_at IS NULL AND (n.owner_id=$3 OR s.user_id=$3)`, id, actor.TenantID, actor.ID).Scan(&key, &name); err != nil {
		errorJSON(writer, 404, "NOT_FOUND", "file not found")
		return
	}
	client, bucket, err := recordingObjectClient()
	if err != nil {
		errorJSON(writer, 503, "STORAGE_UNAVAILABLE", "drive storage is unavailable")
		return
	}
	url, err := client.PresignedGetObject(request.Context(), bucket, key, 5*time.Minute, nil)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not issue download")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "drive.file.downloaded", "drive_node", id, nil)
	respondJSON(writer, 200, map[string]any{"url": url.String(), "name": name, "expiresAt": time.Now().Add(5 * time.Minute).UTC()})
}

func (api *API) driveNodeUpdate(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	id := request.PathValue("nodeID")
	if !api.canEditDriveNode(request, actor, id) {
		errorJSON(writer, 403, "FORBIDDEN", "edit permission is required")
		return
	}
	if request.Method == http.MethodDelete {
		_, err := api.database.ExecContext(request.Context(), `UPDATE drive_nodes SET deleted_at=NOW(),retention_until=GREATEST(COALESCE(retention_until,NOW()),NOW()+COALESCE((SELECT drive_trash_retention_days FROM tenant_governance_policies WHERE tenant_id=$2),30)*INTERVAL '1 day'),updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, id, actor.TenantID)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not delete node")
			return
		}
		_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "drive.node.deleted", "drive_node", id, nil)
		writer.WriteHeader(204)
		return
	}
	var input struct {
		Name     string  `json:"name"`
		ParentID *string `json:"parentId"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", err.Error())
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 255 {
		errorJSON(writer, 400, "INVALID_INPUT", "name is invalid")
		return
	}
	_, err := api.database.ExecContext(request.Context(), `UPDATE drive_nodes SET name=$1,parent_id=$2,version=version+1,updated_at=NOW() WHERE id=$3 AND tenant_id=$4`, input.Name, input.ParentID, id, actor.TenantID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not update node")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "drive.node.updated", "drive_node", id, nil)
	writer.WriteHeader(204)
}

func (api *API) driveShare(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	id, userID := request.PathValue("nodeID"), request.PathValue("userID")
	var owner bool
	_ = api.database.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM drive_nodes WHERE id=$1 AND tenant_id=$2 AND owner_id=$3 AND deleted_at IS NULL)`, id, actor.TenantID, actor.ID).Scan(&owner)
	if !owner {
		errorJSON(writer, 403, "FORBIDDEN", "only the owner can share this node")
		return
	}
	var input struct {
		Permission string `json:"permission"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", err.Error())
		return
	}
	input.Permission = strings.ToUpper(strings.TrimSpace(input.Permission))
	if input.Permission != "VIEW" && input.Permission != "EDIT" {
		errorJSON(writer, 400, "INVALID_INPUT", "permission is invalid")
		return
	}
	result, err := api.database.ExecContext(request.Context(), `INSERT INTO drive_shares(node_id,tenant_id,user_id,permission) SELECT $1,$2,id,$4 FROM users WHERE id=$3 AND tenant_id=$2 AND status='ACTIVE' ON CONFLICT(node_id,user_id) DO UPDATE SET permission=EXCLUDED.permission`, id, actor.TenantID, userID, input.Permission)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not share node")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		errorJSON(writer, 404, "NOT_FOUND", "user not found")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "drive.node.shared", "drive_node", id, map[string]any{"userId": userID, "permission": input.Permission})
	writer.WriteHeader(204)
}

func (api *API) canEditDriveNode(request *http.Request, actor currentUser, id string) bool {
	var allowed bool
	return api.database.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM drive_nodes n LEFT JOIN drive_shares s ON s.node_id=n.id AND s.tenant_id=n.tenant_id AND s.user_id=$3 WHERE n.id=$1 AND n.tenant_id=$2 AND n.deleted_at IS NULL AND (n.owner_id=$3 OR s.permission='EDIT'))`, id, actor.TenantID, actor.ID).Scan(&allowed) == nil && allowed
}

func (api *API) cleanupDriveRetention(ctx context.Context, tenantID string) int64 {
	client, bucket, err := recordingObjectClient()
	if err != nil {
		return 0
	}
	rows, err := api.database.QueryContext(ctx, `SELECT n.id,n.object_key FROM drive_nodes n WHERE n.tenant_id=$1 AND n.kind='FILE' AND n.deleted_at IS NOT NULL AND n.retention_until<=NOW() AND NOT EXISTS(SELECT 1 FROM legal_hold_resources hr JOIN legal_holds h ON h.id=hr.hold_id AND h.tenant_id=hr.tenant_id WHERE hr.tenant_id=n.tenant_id AND hr.resource_type='DRIVE_FILE' AND hr.resource_id=n.id AND h.status='ACTIVE') ORDER BY n.retention_until LIMIT 50`, tenantID)
	if err != nil {
		return 0
	}
	defer rows.Close()
	type expired struct{ id, key string }
	items := make([]expired, 0)
	for rows.Next() {
		var item expired
		if rows.Scan(&item.id, &item.key) == nil {
			items = append(items, item)
		}
	}
	var removed int64
	for _, item := range items {
		if client.RemoveObject(ctx, bucket, item.key, minio.RemoveObjectOptions{}) == nil {
			result, _ := api.database.ExecContext(ctx, `DELETE FROM drive_nodes n WHERE n.id=$1 AND n.tenant_id=$2 AND n.deleted_at IS NOT NULL AND n.retention_until<=NOW() AND NOT EXISTS(SELECT 1 FROM legal_hold_resources hr JOIN legal_holds h ON h.id=hr.hold_id AND h.tenant_id=hr.tenant_id WHERE hr.tenant_id=n.tenant_id AND hr.resource_type='DRIVE_FILE' AND hr.resource_id=n.id AND h.status='ACTIVE')`, item.id, tenantID)
			if result != nil {
				count, _ := result.RowsAffected()
				removed += count
			}
		}
	}
	return removed
}

func (api *API) driveVersions(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	id := request.PathValue("nodeID")
	if !api.canEditDriveNode(request, actor, id) {
		errorJSON(writer, 403, "FORBIDDEN", "edit permission is required")
		return
	}
	if request.Method == http.MethodGet {
		rows, err := api.database.QueryContext(request.Context(), `SELECT version,content_type,size_bytes,created_at FROM drive_versions WHERE node_id=$1 AND tenant_id=$2 UNION ALL SELECT version,content_type,size_bytes,updated_at FROM drive_nodes WHERE id=$1 AND tenant_id=$2 AND kind='FILE' ORDER BY version DESC`, id, actor.TenantID)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load versions")
			return
		}
		defer rows.Close()
		items := make([]driveVersion, 0)
		for rows.Next() {
			var item driveVersion
			if err := rows.Scan(&item.Version, &item.ContentType, &item.SizeBytes, &item.CreatedAt); err != nil {
				errorJSON(writer, 500, "INTERNAL_ERROR", "could not load versions")
				return
			}
			items = append(items, item)
		}
		respondJSON(writer, 200, map[string]any{"versions": items})
		return
	}
	if err := request.ParseMultipartForm(maxDriveUpload); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", "invalid version upload")
		return
	}
	file, header, err := request.FormFile("file")
	if err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", "file is required")
		return
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > maxDriveUpload {
		errorJSON(writer, 413, "FILE_TOO_LARGE", "file must be 100 MB or smaller")
		return
	}
	var oldKey, oldType, name string
	var oldSize int64
	var oldVersion int
	if err = api.database.QueryRowContext(request.Context(), `SELECT object_key,content_type,size_bytes,version,name FROM drive_nodes WHERE id=$1 AND tenant_id=$2 AND kind='FILE' AND deleted_at IS NULL`, id, actor.TenantID).Scan(&oldKey, &oldType, &oldSize, &oldVersion, &name); err != nil {
		errorJSON(writer, 404, "NOT_FOUND", "file not found")
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	newKey := fmt.Sprintf("drive/%s/%d-%s", actor.TenantID, time.Now().UnixNano(), filepath.Base(header.Filename))
	client, bucket, err := recordingObjectClient()
	if err != nil {
		errorJSON(writer, 503, "STORAGE_UNAVAILABLE", "drive storage is unavailable")
		return
	}
	if _, err = client.PutObject(request.Context(), bucket, newKey, file, header.Size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		errorJSON(writer, 502, "STORAGE_ERROR", "could not store version")
		return
	}
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		_ = client.RemoveObject(request.Context(), bucket, newKey, minio.RemoveObjectOptions{})
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not save version")
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(request.Context(), `INSERT INTO drive_versions(tenant_id,node_id,version,object_key,content_type,size_bytes,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(node_id,version) DO NOTHING`, actor.TenantID, id, oldVersion, oldKey, oldType, oldSize, actor.ID)
	if err == nil {
		_, err = tx.ExecContext(request.Context(), `UPDATE drive_nodes SET object_key=$1,content_type=$2,size_bytes=$3,version=version+1,updated_at=NOW() WHERE id=$4 AND tenant_id=$5`, newKey, contentType, header.Size, id, actor.TenantID)
	}
	if err != nil || tx.Commit() != nil {
		_ = client.RemoveObject(request.Context(), bucket, newKey, minio.RemoveObjectOptions{})
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not save version")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "drive.file.version.created", "drive_node", id, map[string]any{"version": oldVersion + 1})
	respondJSON(writer, 201, map[string]any{"version": oldVersion + 1, "name": name})
}

func (api *API) driveVersionRestore(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	id := request.PathValue("nodeID")
	version, err := strconv.Atoi(request.PathValue("version"))
	if err != nil || version < 1 {
		errorJSON(writer, 400, "INVALID_INPUT", "version is invalid")
		return
	}
	if !api.canEditDriveNode(request, actor, id) {
		errorJSON(writer, 403, "FORBIDDEN", "edit permission is required")
		return
	}
	var restoreKey, restoreType, currentKey, currentType string
	var restoreSize, currentSize int64
	var currentVersion int
	err = api.database.QueryRowContext(request.Context(), `SELECT v.object_key,v.content_type,v.size_bytes,n.object_key,n.content_type,n.size_bytes,n.version FROM drive_versions v JOIN drive_nodes n ON n.id=v.node_id AND n.tenant_id=v.tenant_id WHERE v.node_id=$1 AND v.tenant_id=$2 AND v.version=$3 AND n.deleted_at IS NULL`, id, actor.TenantID, version).Scan(&restoreKey, &restoreType, &restoreSize, &currentKey, &currentType, &currentSize, &currentVersion)
	if err != nil {
		errorJSON(writer, 404, "NOT_FOUND", "version not found")
		return
	}
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not restore version")
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(request.Context(), `INSERT INTO drive_versions(tenant_id,node_id,version,object_key,content_type,size_bytes,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(node_id,version) DO NOTHING`, actor.TenantID, id, currentVersion, currentKey, currentType, currentSize, actor.ID)
	if err == nil {
		_, err = tx.ExecContext(request.Context(), `UPDATE drive_nodes SET object_key=$1,content_type=$2,size_bytes=$3,version=version+1,updated_at=NOW() WHERE id=$4 AND tenant_id=$5`, restoreKey, restoreType, restoreSize, id, actor.TenantID)
	}
	if err != nil || tx.Commit() != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not restore version")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "drive.file.version.restored", "drive_node", id, map[string]any{"restoredFrom": version, "newVersion": currentVersion + 1})
	respondJSON(writer, 200, map[string]any{"version": currentVersion + 1})
}
