package httpapi

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type recordingResponse struct {
	ID           string     `json:"id"`
	Status       string     `json:"status"`
	EgressID     *string    `json:"egressId,omitempty"`
	StartedAt    time.Time  `json:"startedAt"`
	StoppedAt    *time.Time `json:"stoppedAt,omitempty"`
	MeetingTitle string     `json:"meetingTitle,omitempty"`
	JoinCode     string     `json:"joinCode,omitempty"`
	CanDelete    bool       `json:"canDelete,omitempty"`
}

// recordingLibrary is intentionally independent from meeting pagination. A
// recording must remain discoverable after its meeting falls off the first
// page of the meeting list.
func (api *API) recordingLibrary(writer http.ResponseWriter, request *http.Request, user currentUser) {
	api.reconcileRecordingStatuses(request, user.TenantID)
	rows, err := api.database.QueryContext(request.Context(), `
		SELECT r.id,r.status,r.egress_id,r.started_at,r.stopped_at,m.title,m.join_code,
		       ($3 OR m.host_id=$4)
		FROM recordings r JOIN meetings m ON m.id=r.meeting_id AND m.tenant_id=r.tenant_id
		WHERE r.tenant_id=$1 AND r.retention_expired_at IS NULL AND (
		  $2 OR m.host_id=$4 OR r.started_by=$4 OR EXISTS (
		    SELECT 1 FROM recording_access_grants access
		    WHERE access.recording_id=r.id AND access.tenant_id=r.tenant_id AND access.user_id=$4
		  )
		)
		ORDER BY r.created_at DESC LIMIT 200`, user.TenantID, user.Role.isWorkspaceAdmin(), user.Role.isWorkspaceAdmin(), user.ID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load recording library")
		return
	}
	defer rows.Close()
	items := []recordingResponse{}
	for rows.Next() {
		var item recordingResponse
		if err = rows.Scan(&item.ID, &item.Status, &item.EgressID, &item.StartedAt, &item.StoppedAt, &item.MeetingTitle, &item.JoinCode, &item.CanDelete); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not read recording library")
			return
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not read recording library")
		return
	}
	respondJSON(writer, http.StatusOK, map[string]any{"recordings": items})
}

func liveKitEgressClient() (*lksdk.EgressClient, error) {
	key, secret := os.Getenv("LIVEKIT_API_KEY"), os.Getenv("LIVEKIT_API_SECRET")
	if key == "" || secret == "" {
		return nil, fmt.Errorf("livekit credentials missing")
	}
	return lksdk.NewEgressClient(envOr("LIVEKIT_API_URL", "http://localhost:7880"), key, secret), nil
}

func (api *API) recordings(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeeting(request, user)
	if err != nil {
		errorJSON(writer, http.StatusNotFound, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	switch request.Method {
	case http.MethodGet:
		api.listRecordings(writer, request, user, meeting)
	case http.MethodPost:
		if meeting.HostID != user.ID {
			errorJSON(writer, http.StatusForbidden, "HOST_REQUIRED", "only the host can control recording")
			return
		}
		api.startRecording(writer, request, user, meeting)
	}
}

func (api *API) startRecording(writer http.ResponseWriter, request *http.Request, user currentUser, meeting meetingResponse) {
	if err := api.enforceTenantQuota(request.Context(), user.TenantID, "recordings", 1); err != nil {
		if !respondEntitlementError(writer, err) {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not verify workspace quota")
		}
		return
	}
	policy, policyErr := api.loadMeetingPolicy(request.Context(), user.TenantID)
	if policyErr != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meeting policy")
		return
	}
	if !policy.RecordingEnabled {
		errorJSON(writer, http.StatusForbidden, "RECORDING_DISABLED", "recording is disabled by workspace policy")
		return
	}
	if meeting.Status == "ENDED" {
		errorJSON(writer, http.StatusConflict, "MEETING_ENDED", "an ended meeting cannot be recorded")
		return
	}
	// Repair a stale DB state first when LiveKit has already completed or
	// aborted the previous egress (for example after the room closed).
	api.reconcileRecordingStatuses(request, user.TenantID)
	id, err := randomToken(12)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not create recording")
		return
	}
	objectKey := fmt.Sprintf("tenants/%s/meetings/%s/%s.mp4", user.TenantID, meeting.ID, id)
	var recordingID string
	err = api.database.QueryRowContext(request.Context(), `INSERT INTO recordings (tenant_id,meeting_id,started_by,object_key) VALUES ($1,$2,$3,$4) RETURNING id`, user.TenantID, meeting.ID, user.ID, objectKey).Scan(&recordingID)
	if err != nil {
		errorJSON(writer, http.StatusConflict, "RECORDING_ACTIVE", "this meeting already has an active recording")
		return
	}
	client, err := liveKitEgressClient()
	if err != nil {
		api.failRecording(request, user.TenantID, recordingID)
		errorJSON(writer, 503, "RECORDING_UNAVAILABLE", "recording service is not configured")
		return
	}
	info, err := client.StartRoomCompositeEgress(request.Context(), &livekit.RoomCompositeEgressRequest{
		RoomName: meeting.RoomName, Layout: "grid",
		FileOutputs: []*livekit.EncodedFileOutput{{FileType: livekit.EncodedFileType_MP4, Filepath: objectKey, Output: &livekit.EncodedFileOutput_S3{S3: &livekit.S3Upload{
			AccessKey: envOr("MINIO_ROOT_USER", "xpace-local"), Secret: os.Getenv("MINIO_ROOT_PASSWORD"),
			Endpoint: envOr("RECORDING_S3_ENDPOINT", "http://minio:9000"), Bucket: envOr("RECORDING_S3_BUCKET", "xpace-recordings"), ForcePathStyle: true,
		}}}},
	})
	if err != nil {
		api.failRecording(request, user.TenantID, recordingID)
		errorJSON(writer, 502, "RECORDING_START_FAILED", "recording worker could not start")
		return
	}
	_, err = api.database.ExecContext(request.Context(), `UPDATE recordings SET egress_id=$1,status='RECORDING',updated_at=NOW() WHERE id=$2 AND tenant_id=$3`, info.EgressId, recordingID, user.TenantID)
	if err != nil {
		_, _ = client.StopEgress(request.Context(), &livekit.StopEgressRequest{EgressId: info.EgressId})
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not save recording state")
		return
	}
	api.auditRecording(request, user, "recording.start", recordingID)
	respondJSON(writer, http.StatusCreated, map[string]any{"recording": map[string]any{"id": recordingID, "status": "RECORDING", "startedAt": time.Now().UTC()}})
}

func (api *API) stopRecording(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeeting(request, user)
	if err != nil {
		errorJSON(writer, 404, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	if meeting.HostID != user.ID {
		errorJSON(writer, 403, "HOST_REQUIRED", "only the host can control recording")
		return
	}
	var id, egressID string
	err = api.database.QueryRowContext(request.Context(), `UPDATE recordings SET status='STOPPING',updated_at=NOW() WHERE meeting_id=$1 AND tenant_id=$2 AND status IN ('STARTING','RECORDING') RETURNING id,egress_id`, meeting.ID, user.TenantID).Scan(&id, &egressID)
	if err == sql.ErrNoRows {
		errorJSON(writer, 409, "NO_ACTIVE_RECORDING", "this meeting is not being recorded")
		return
	}
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not stop recording")
		return
	}
	client, err := liveKitEgressClient()
	if err == nil {
		_, err = client.StopEgress(request.Context(), &livekit.StopEgressRequest{EgressId: egressID})
	}
	if err != nil {
		_, _ = api.database.ExecContext(request.Context(), `UPDATE recordings SET status='RECORDING',updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, id, user.TenantID)
		errorJSON(writer, 502, "RECORDING_STOP_FAILED", "recording worker could not stop")
		return
	}
	_, _ = api.database.ExecContext(request.Context(), `UPDATE recordings SET status='READY',stopped_at=NOW(),updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, id, user.TenantID)
	api.auditRecording(request, user, "recording.stop", id)
	respondJSON(writer, 200, map[string]string{"status": "READY", "id": id})
}

func (api *API) listRecordings(writer http.ResponseWriter, request *http.Request, user currentUser, meeting meetingResponse) {
	canViewAll := user.Role.isWorkspaceAdmin() || user.ID == meeting.HostID
	rows, err := api.database.QueryContext(request.Context(), `
		SELECT r.id,r.status,r.egress_id,r.started_at,r.stopped_at
		FROM recordings r
		WHERE r.meeting_id=$1 AND r.tenant_id=$2 AND r.retention_expired_at IS NULL AND (
		  $3 OR r.started_by=$4 OR EXISTS (
		    SELECT 1 FROM recording_access_grants grant_access
		    WHERE grant_access.recording_id=r.id AND grant_access.tenant_id=r.tenant_id AND grant_access.user_id=$4
		  )
		)
		ORDER BY r.created_at DESC`, meeting.ID, user.TenantID, canViewAll, user.ID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load recordings")
		return
	}
	defer rows.Close()
	items := []recordingResponse{}
	for rows.Next() {
		var item recordingResponse
		if err = rows.Scan(&item.ID, &item.Status, &item.EgressID, &item.StartedAt, &item.StoppedAt); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not read recordings")
			return
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not read recordings")
		return
	}
	respondJSON(writer, 200, map[string]any{"recordings": items})
}

func (api *API) recordingAccess(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeeting(request, user)
	if err != nil {
		errorJSON(writer, http.StatusNotFound, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	if !canManageRecordingAccess(meeting, user) {
		errorJSON(writer, http.StatusForbidden, "HOST_REQUIRED", "only the host or a workspace admin can manage recording access")
		return
	}
	recordingID, targetUserID := request.PathValue("recordingID"), request.PathValue("userID")
	var exists bool
	err = api.database.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM recordings WHERE id=$1 AND meeting_id=$2 AND tenant_id=$3) AND EXISTS(SELECT 1 FROM users WHERE id=$4 AND tenant_id=$3 AND status='ACTIVE')`, recordingID, meeting.ID, user.TenantID, targetUserID).Scan(&exists)
	if err != nil || !exists {
		errorJSON(writer, http.StatusNotFound, "ACCESS_TARGET_NOT_FOUND", "recording or target user was not found")
		return
	}
	action := "recording.access.grant"
	if request.Method == http.MethodPut {
		_, err = api.database.ExecContext(request.Context(), `INSERT INTO recording_access_grants (recording_id,tenant_id,user_id,granted_by) VALUES ($1,$2,$3,$4) ON CONFLICT (recording_id,user_id) DO UPDATE SET granted_by=EXCLUDED.granted_by,created_at=NOW() WHERE recording_access_grants.tenant_id=EXCLUDED.tenant_id`, recordingID, user.TenantID, targetUserID, user.ID)
	} else {
		action = "recording.access.revoke"
		_, err = api.database.ExecContext(request.Context(), `DELETE FROM recording_access_grants WHERE recording_id=$1 AND tenant_id=$2 AND user_id=$3`, recordingID, user.TenantID, targetUserID)
	}
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update recording access")
		return
	}
	api.auditRecording(request, user, action, recordingID)
	respondJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (api *API) recordingDownload(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeeting(request, user)
	if err != nil {
		errorJSON(writer, http.StatusNotFound, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	recordingID := request.PathValue("recordingID")
	var objectKey string
	err = api.database.QueryRowContext(request.Context(), `
		SELECT r.object_key FROM recordings r
		WHERE r.id=$1 AND r.meeting_id=$2 AND r.tenant_id=$3 AND r.status='READY' AND r.retention_expired_at IS NULL AND (
		  $4 OR $5=$6 OR r.started_by=$5 OR EXISTS (
		    SELECT 1 FROM recording_access_grants access
		    WHERE access.recording_id=r.id AND access.tenant_id=r.tenant_id AND access.user_id=$5
		  )
		)`, recordingID, meeting.ID, user.TenantID, user.Role.isWorkspaceAdmin(), user.ID, meeting.HostID).Scan(&objectKey)
	if err != nil {
		errorJSON(writer, http.StatusNotFound, "RECORDING_NOT_FOUND", "recording is unavailable or access was not granted")
		return
	}
	client, bucket, err := recordingObjectClient()
	if err != nil {
		errorJSON(writer, http.StatusServiceUnavailable, "RECORDING_UNAVAILABLE", "recording storage is not configured")
		return
	}
	expiresIn := 5 * time.Minute
	parameters := make(url.Values)
	parameters.Set("response-content-disposition", `attachment; filename="`+filepath.Base(objectKey)+`"`)
	downloadURL, err := client.PresignedGetObject(request.Context(), bucket, objectKey, expiresIn, parameters)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not issue recording download")
		return
	}
	api.auditRecording(request, user, "recording.download.issue", recordingID)
	respondJSON(writer, http.StatusOK, map[string]any{"url": downloadURL.String(), "expiresAt": time.Now().Add(expiresIn).UTC()})
}

// recordingLibraryFile resolves a recording directly from its tenant-scoped
// library ID. It avoids coupling playback/download to a meeting-list route.
func (api *API) recordingLibraryFile(writer http.ResponseWriter, request *http.Request, user currentUser) {
	recordingID := request.PathValue("recordingID")
	var objectKey string
	err := api.database.QueryRowContext(request.Context(), `
		SELECT r.object_key FROM recordings r
		JOIN meetings m ON m.id=r.meeting_id AND m.tenant_id=r.tenant_id
		WHERE r.id=$1 AND r.tenant_id=$2 AND r.status='READY' AND r.retention_expired_at IS NULL AND (
		  $3 OR m.host_id=$4 OR r.started_by=$4 OR EXISTS (
		    SELECT 1 FROM recording_access_grants access
		    WHERE access.recording_id=r.id AND access.tenant_id=r.tenant_id AND access.user_id=$4
		  )
		)`, recordingID, user.TenantID, user.Role.isWorkspaceAdmin(), user.ID).Scan(&objectKey)
	if err != nil {
		errorJSON(writer, http.StatusNotFound, "RECORDING_NOT_FOUND", "recording is unavailable or access was not granted")
		return
	}
	client, bucket, err := recordingObjectClient()
	if err != nil {
		errorJSON(writer, http.StatusServiceUnavailable, "RECORDING_UNAVAILABLE", "recording storage is not configured")
		return
	}
	disposition := "attachment"
	if request.URL.Query().Get("disposition") == "inline" {
		disposition = "inline"
	}
	expiresIn := 15 * time.Minute
	parameters := make(url.Values)
	parameters.Set("response-content-disposition", disposition+`; filename="`+filepath.Base(objectKey)+`"`)
	parameters.Set("response-content-type", "video/mp4")
	fileURL, err := client.PresignedGetObject(request.Context(), bucket, objectKey, expiresIn, parameters)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not issue recording file URL")
		return
	}
	action := "recording.download.issue"
	if disposition == "inline" {
		action = "recording.preview.issue"
	}
	api.auditRecording(request, user, action, recordingID)
	respondJSON(writer, http.StatusOK, map[string]any{"url": fileURL.String(), "expiresAt": time.Now().Add(expiresIn).UTC()})
}

func (api *API) deleteLibraryRecording(writer http.ResponseWriter, request *http.Request, user currentUser) {
	recordingID := request.PathValue("recordingID")
	var meetingID, objectKey, status string
	var held, canDelete bool
	err := api.database.QueryRowContext(request.Context(), `
		SELECT r.meeting_id,r.object_key,r.status,
		       EXISTS(SELECT 1 FROM legal_hold_resources hr JOIN legal_holds h ON h.id=hr.hold_id AND h.tenant_id=hr.tenant_id WHERE hr.tenant_id=r.tenant_id AND hr.resource_type='RECORDING' AND hr.resource_id=r.id AND h.status='ACTIVE'),
		       ($3 OR m.host_id=$4)
		FROM recordings r JOIN meetings m ON m.id=r.meeting_id AND m.tenant_id=r.tenant_id
		WHERE r.id=$1 AND r.tenant_id=$2 AND r.retention_expired_at IS NULL`, recordingID, user.TenantID, user.Role.isWorkspaceAdmin(), user.ID).Scan(&meetingID, &objectKey, &status, &held, &canDelete)
	if err != nil {
		errorJSON(writer, http.StatusNotFound, "RECORDING_NOT_FOUND", "recording was not found")
		return
	}
	if !canDelete {
		errorJSON(writer, http.StatusForbidden, "HOST_REQUIRED", "only the host or a workspace admin can delete a recording")
		return
	}
	if held {
		errorJSON(writer, http.StatusConflict, "LEGAL_HOLD_ACTIVE", "recording cannot be deleted while a legal hold is active")
		return
	}
	if status == "STARTING" || status == "RECORDING" || status == "STOPPING" {
		errorJSON(writer, http.StatusConflict, "RECORDING_ACTIVE", "stop the recording before deleting it")
		return
	}
	client, bucket, clientErr := recordingInternalObjectClient()
	if clientErr != nil {
		errorJSON(writer, http.StatusServiceUnavailable, "RECORDING_UNAVAILABLE", "recording storage is not configured")
		return
	}
	if err = client.RemoveObject(request.Context(), bucket, objectKey, minio.RemoveObjectOptions{}); err != nil {
		errorJSON(writer, http.StatusBadGateway, "RECORDING_DELETE_FAILED", "recording file could not be deleted")
		return
	}
	result, err := api.database.ExecContext(request.Context(), `UPDATE recordings SET retention_expired_at=NOW(),storage_deleted_at=NOW(),size_bytes=0,updated_at=NOW() WHERE id=$1 AND tenant_id=$2 AND meeting_id=$3 AND retention_expired_at IS NULL`, recordingID, user.TenantID, meetingID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "recording metadata could not be deleted")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		errorJSON(writer, http.StatusNotFound, "RECORDING_NOT_FOUND", "recording was not found")
		return
	}
	api.auditRecording(request, user, "recording.delete", recordingID)
	respondJSON(writer, http.StatusOK, map[string]string{"status": "deleted"})
}

func (api *API) deleteRecording(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeeting(request, user)
	if err != nil {
		errorJSON(writer, http.StatusNotFound, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	if !canManageRecordingAccess(meeting, user) {
		errorJSON(writer, http.StatusForbidden, "HOST_REQUIRED", "only the host or a workspace admin can delete a recording")
		return
	}
	recordingID := request.PathValue("recordingID")
	var objectKey, status string
	var held bool
	err = api.database.QueryRowContext(request.Context(), `
		SELECT r.object_key,r.status,EXISTS(
		  SELECT 1 FROM legal_hold_resources hr JOIN legal_holds h ON h.id=hr.hold_id AND h.tenant_id=hr.tenant_id
		  WHERE hr.tenant_id=r.tenant_id AND hr.resource_type='RECORDING' AND hr.resource_id=r.id AND h.status='ACTIVE'
		) FROM recordings r WHERE r.id=$1 AND r.meeting_id=$2 AND r.tenant_id=$3 AND r.retention_expired_at IS NULL`, recordingID, meeting.ID, user.TenantID).Scan(&objectKey, &status, &held)
	if err != nil {
		errorJSON(writer, http.StatusNotFound, "RECORDING_NOT_FOUND", "recording was not found")
		return
	}
	if held {
		errorJSON(writer, http.StatusConflict, "LEGAL_HOLD_ACTIVE", "recording cannot be deleted while a legal hold is active")
		return
	}
	if status == "STARTING" || status == "RECORDING" || status == "STOPPING" {
		errorJSON(writer, http.StatusConflict, "RECORDING_ACTIVE", "stop the recording before deleting it")
		return
	}
	client, bucket, clientErr := recordingInternalObjectClient()
	if clientErr != nil {
		errorJSON(writer, http.StatusServiceUnavailable, "RECORDING_UNAVAILABLE", "recording storage is not configured")
		return
	}
	if err = client.RemoveObject(request.Context(), bucket, objectKey, minio.RemoveObjectOptions{}); err != nil {
		errorJSON(writer, http.StatusBadGateway, "RECORDING_DELETE_FAILED", "recording file could not be deleted")
		return
	}
	result, err := api.database.ExecContext(request.Context(), `UPDATE recordings SET retention_expired_at=NOW(),storage_deleted_at=NOW(),size_bytes=0,updated_at=NOW() WHERE id=$1 AND tenant_id=$2 AND retention_expired_at IS NULL`, recordingID, user.TenantID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "recording metadata could not be deleted")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		errorJSON(writer, http.StatusNotFound, "RECORDING_NOT_FOUND", "recording was not found")
		return
	}
	api.auditRecording(request, user, "recording.delete", recordingID)
	respondJSON(writer, http.StatusOK, map[string]string{"status": "deleted"})
}

func recordingObjectClient() (*minio.Client, string, error) {
	return newRecordingObjectClient(envOr("RECORDING_S3_PUBLIC_ENDPOINT", "http://localhost:9000"))
}

func recordingInternalObjectClient() (*minio.Client, string, error) {
	return newRecordingObjectClient(envOr("RECORDING_S3_ENDPOINT", "http://minio:9000"))
}

func newRecordingObjectClient(endpoint string) (*minio.Client, string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.Path != "" && parsed.Path != "/" || parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, "", fmt.Errorf("invalid recording public endpoint")
	}
	accessKey, secret := os.Getenv("MINIO_ROOT_USER"), os.Getenv("MINIO_ROOT_PASSWORD")
	if strings.TrimSpace(accessKey) == "" || strings.TrimSpace(secret) == "" {
		return nil, "", fmt.Errorf("recording credentials missing")
	}
	client, err := minio.New(parsed.Host, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secret, ""), Secure: parsed.Scheme == "https"})
	return client, envOr("RECORDING_S3_BUCKET", "xpace-recordings"), err
}

func (api *API) reconcileRecordingStatuses(request *http.Request, tenantID string) {
	egressClient, _ := liveKitEgressClient()
	objectClient, bucket, _ := recordingInternalObjectClient()
	rows, err := api.database.QueryContext(request.Context(), `
		SELECT id,COALESCE(egress_id,''),object_key,status
		FROM recordings
		WHERE tenant_id=$1 AND retention_expired_at IS NULL AND (
		  status IN ('STARTING','RECORDING','STOPPING') OR
		  (status='READY' AND COALESCE(size_bytes,0)=0)
		)`, tenantID)
	if err != nil {
		return
	}
	type staleRecording struct{ id, egressID, objectKey, status string }
	items := []staleRecording{}
	for rows.Next() {
		var item staleRecording
		if rows.Scan(&item.id, &item.egressID, &item.objectKey, &item.status) == nil {
			items = append(items, item)
		}
	}
	rows.Close()
	for _, item := range items {
		if egressClient != nil && item.egressID != "" && item.status != "READY" {
			response, listErr := egressClient.ListEgress(request.Context(), &livekit.ListEgressRequest{EgressId: item.egressID})
			if listErr == nil && len(response.Items) > 0 {
				info := response.Items[0]
				switch info.Status {
				case livekit.EgressStatus_EGRESS_COMPLETE:
					size, duration := completedRecordingMetrics(info, item.objectKey)
					_, _ = api.database.ExecContext(request.Context(), `UPDATE recordings SET status='READY',stopped_at=COALESCE(stopped_at,NOW()),size_bytes=CASE WHEN $3>0 THEN $3 ELSE size_bytes END,duration_seconds=CASE WHEN $4>0 THEN $4 ELSE duration_seconds END,updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, item.id, tenantID, size, duration)
					if size > 0 {
						continue
					}
				case livekit.EgressStatus_EGRESS_FAILED, livekit.EgressStatus_EGRESS_ABORTED, livekit.EgressStatus_EGRESS_LIMIT_REACHED:
					_, _ = api.database.ExecContext(request.Context(), `UPDATE recordings SET status='FAILED',stopped_at=COALESCE(stopped_at,NOW()),updated_at=NOW() WHERE id=$1 AND tenant_id=$2 AND status IN ('STARTING','RECORDING','STOPPING')`, item.id, tenantID)
					continue
				}
			}
		}
		// A completed S3 object is the durable source of truth when LiveKit no
		// longer retains an old egress result. Multipart uploads are invisible
		// until completion, so a successful StatObject is safe to mark READY.
		if objectClient != nil {
			object, statErr := objectClient.StatObject(request.Context(), bucket, item.objectKey, minio.StatObjectOptions{})
			if statErr == nil && object.Size > 0 {
				_, _ = api.database.ExecContext(request.Context(), `UPDATE recordings SET status='READY',stopped_at=COALESCE(stopped_at,$3),size_bytes=$4,updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, item.id, tenantID, object.LastModified, object.Size)
			}
		}
	}
}

func completedRecordingMetrics(info *livekit.EgressInfo, objectKey string) (int64, int64) {
	for _, file := range info.GetFileResults() {
		if file == nil || file.GetFilename() != "" && file.GetFilename() != objectKey {
			continue
		}
		durationSeconds := file.GetDuration() / int64(time.Second)
		return file.GetSize(), durationSeconds
	}
	return 0, 0
}

func canManageRecordingAccess(meeting meetingResponse, user currentUser) bool {
	return user.Role.isWorkspaceAdmin() || meeting.HostID == user.ID
}

func (api *API) failRecording(request *http.Request, tenantID, id string) {
	_, _ = api.database.ExecContext(request.Context(), `UPDATE recordings SET status='FAILED',updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, id, tenantID)
}
func (api *API) auditRecording(request *http.Request, user currentUser, action, id string) {
	_ = api.writeAuditEvent(request.Context(), request, user.TenantID, user.ID, action, "recording", id, nil)
}
