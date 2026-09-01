package httpapi

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
)

const maxChatAttachment = 25 << 20

type chatAttachment struct {
	ID           string    `json:"id"`
	MessageID    string    `json:"messageId"`
	OriginalName string    `json:"originalName"`
	ContentType  string    `json:"contentType"`
	SizeBytes    int64     `json:"sizeBytes"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (api *API) loadChatAttachments(request *http.Request, tenantID, messageID string) ([]chatAttachment, error) {
	rows, err := api.database.QueryContext(request.Context(), `SELECT id,message_id,original_name,content_type,size_bytes,created_at FROM chat_attachments WHERE tenant_id=$1 AND message_id=$2 ORDER BY created_at`, tenantID, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]chatAttachment, 0)
	for rows.Next() {
		var item chatAttachment
		if err := rows.Scan(&item.ID, &item.MessageID, &item.OriginalName, &item.ContentType, &item.SizeBytes, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (api *API) chatAttachment(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	conversationID, messageID := request.PathValue("conversationID"), request.PathValue("messageID")
	var member bool
	if err := api.database.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM chat_members WHERE tenant_id=$1 AND conversation_id=$2 AND user_id=$3)`, actor.TenantID, conversationID, actor.ID).Scan(&member); err != nil || !member {
		errorJSON(writer, http.StatusNotFound, "NOT_FOUND", "conversation not found")
		return
	}
	var validMessage bool
	if err := api.database.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM chat_messages WHERE tenant_id=$1 AND conversation_id=$2 AND id=$3)`, actor.TenantID, conversationID, messageID).Scan(&validMessage); err != nil || !validMessage {
		errorJSON(writer, http.StatusNotFound, "NOT_FOUND", "message not found")
		return
	}
	client, bucket, err := recordingObjectClient()
	if err != nil {
		errorJSON(writer, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "attachment storage is not configured")
		return
	}
	switch request.Method {
	case http.MethodGet:
		rows, err := api.database.QueryContext(request.Context(), `SELECT id,message_id,original_name,content_type,size_bytes,created_at FROM chat_attachments WHERE tenant_id=$1 AND conversation_id=$2 AND message_id=$3 ORDER BY created_at`, actor.TenantID, conversationID, messageID)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load attachments")
			return
		}
		defer rows.Close()
		items := make([]chatAttachment, 0)
		for rows.Next() {
			var item chatAttachment
			if err := rows.Scan(&item.ID, &item.MessageID, &item.OriginalName, &item.ContentType, &item.SizeBytes, &item.CreatedAt); err != nil {
				errorJSON(writer, 500, "INTERNAL_ERROR", "could not load attachments")
				return
			}
			items = append(items, item)
		}
		respondJSON(writer, 200, map[string]any{"attachments": items})
	case http.MethodPost:
		if request.ContentLength > maxChatAttachment+1<<20 {
			errorJSON(writer, 413, "FILE_TOO_LARGE", "attachment must be 25 MB or smaller")
			return
		}
		if err := request.ParseMultipartForm(maxChatAttachment); err != nil {
			errorJSON(writer, 400, "INVALID_INPUT", "invalid multipart attachment")
			return
		}
		file, header, err := request.FormFile("file")
		if err != nil {
			errorJSON(writer, 400, "INVALID_INPUT", "file is required")
			return
		}
		defer file.Close()
		if header.Size <= 0 || header.Size > maxChatAttachment {
			errorJSON(writer, 413, "FILE_TOO_LARGE", "attachment must be 25 MB or smaller")
			return
		}
		if err = api.enforceTenantQuota(request.Context(), actor.TenantID, "chatAttachments", header.Size); err != nil {
			if !respondEntitlementError(writer, err) {
				errorJSON(writer, 500, "INTERNAL_ERROR", "could not verify workspace quota")
			}
			return
		}
		name := filepath.Base(strings.TrimSpace(header.Filename))
		if name == "." || name == "" {
			errorJSON(writer, 400, "INVALID_INPUT", "file name is required")
			return
		}
		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		objectKey := fmt.Sprintf("chat/%s/%s/%d-%s", actor.TenantID, conversationID, time.Now().UnixNano(), name)
		if _, err := client.PutObject(request.Context(), bucket, objectKey, file, header.Size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
			errorJSON(writer, 502, "STORAGE_ERROR", "could not store attachment")
			return
		}
		var item chatAttachment
		err = api.database.QueryRowContext(request.Context(), `INSERT INTO chat_attachments (tenant_id,conversation_id,message_id,uploader_id,object_key,original_name,content_type,size_bytes) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id,message_id,original_name,content_type,size_bytes,created_at`, actor.TenantID, conversationID, messageID, actor.ID, objectKey, name, contentType, header.Size).Scan(&item.ID, &item.MessageID, &item.OriginalName, &item.ContentType, &item.SizeBytes, &item.CreatedAt)
		if err != nil {
			_ = client.RemoveObject(request.Context(), bucket, objectKey, minio.RemoveObjectOptions{})
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not save attachment metadata")
			return
		}
		api.chat.publish(conversationID, "attachment", item)
		respondJSON(writer, 201, map[string]any{"attachment": item})
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *API) chatAttachmentDownload(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	conversationID, messageID, attachmentID := request.PathValue("conversationID"), request.PathValue("messageID"), request.PathValue("attachmentID")
	if !api.isChatMember(request, conversationID, actor) {
		errorJSON(writer, 404, "NOT_FOUND", "conversation not found")
		return
	}
	var objectKey, name string
	if err := api.database.QueryRowContext(request.Context(), `SELECT object_key,original_name FROM chat_attachments WHERE id=$1 AND tenant_id=$2 AND conversation_id=$3 AND message_id=$4`, attachmentID, actor.TenantID, conversationID, messageID).Scan(&objectKey, &name); err != nil {
		errorJSON(writer, 404, "NOT_FOUND", "attachment not found")
		return
	}
	client, bucket, err := recordingObjectClient()
	if err != nil {
		errorJSON(writer, 503, "STORAGE_UNAVAILABLE", "attachment storage is not configured")
		return
	}
	url, err := client.PresignedGetObject(request.Context(), bucket, objectKey, 5*time.Minute, nil)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not issue attachment download")
		return
	}
	respondJSON(writer, 200, map[string]any{"url": url.String(), "name": name, "expiresAt": time.Now().Add(5 * time.Minute).UTC()})
}
