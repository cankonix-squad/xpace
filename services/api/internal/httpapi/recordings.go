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
	ID        string     `json:"id"`
	Status    string     `json:"status"`
	EgressID  *string    `json:"egressId,omitempty"`
	StartedAt time.Time  `json:"startedAt"`
	StoppedAt *time.Time `json:"stoppedAt,omitempty"`
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
	rows, err := api.database.QueryContext(request.Context(), `
		SELECT r.id,r.status,r.egress_id,r.started_at,r.stopped_at
		FROM recordings r
		WHERE r.meeting_id=$1 AND r.tenant_id=$2 AND (
		  $3 OR $4=$5 OR r.started_by=$4 OR EXISTS (
		    SELECT 1 FROM recording_access_grants grant_access
		    WHERE grant_access.recording_id=r.id AND grant_access.tenant_id=r.tenant_id AND grant_access.user_id=$4
		  )
		)
		ORDER BY r.created_at DESC`, meeting.ID, user.TenantID, user.Role.isWorkspaceAdmin(), user.ID, meeting.HostID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load recordings")
		return
	}
	defer rows.Close()
	items := []recordingResponse{}
	for rows.Next() {
		var item recordingResponse
		if rows.Scan(&item.ID, &item.Status, &item.EgressID, &item.StartedAt, &item.StoppedAt) == nil {
			items = append(items, item)
		}
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
		WHERE r.id=$1 AND r.meeting_id=$2 AND r.tenant_id=$3 AND r.status='READY' AND (
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

func recordingObjectClient() (*minio.Client, string, error) {
	endpoint := envOr("RECORDING_S3_PUBLIC_ENDPOINT", "http://localhost:9000")
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

func canManageRecordingAccess(meeting meetingResponse, user currentUser) bool {
	return user.Role.isWorkspaceAdmin() || meeting.HostID == user.ID
}

func (api *API) failRecording(request *http.Request, tenantID, id string) {
	_, _ = api.database.ExecContext(request.Context(), `UPDATE recordings SET status='FAILED',updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, id, tenantID)
}
func (api *API) auditRecording(request *http.Request, user currentUser, action, id string) {
	_ = api.writeAuditEvent(request.Context(), request, user.TenantID, user.ID, action, "recording", id, nil)
}
