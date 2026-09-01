package httpapi

import (
	"net/http"
	"time"
)

type adminMeetingSummary struct {
	Total     int `json:"total"`
	Scheduled int `json:"scheduled"`
	Active    int `json:"active"`
	Ended     int `json:"ended"`
}

type adminUsageSummary struct {
	Users                 int   `json:"users"`
	Participants          int   `json:"participants"`
	ActiveParticipants    int   `json:"activeParticipants"`
	Recordings            int   `json:"recordings"`
	RecordingDurationSecs int64 `json:"recordingDurationSeconds"`
	RecordingStorageBytes int64 `json:"recordingStorageBytes"`
}

type adminOperationalSummary struct {
	ActiveRooms         int   `json:"activeRooms"`
	WaitingParticipants int   `json:"waitingParticipants"`
	JoinedParticipants  int   `json:"joinedParticipants"`
	FailedRecordings    int   `json:"failedRecordings"`
	DriveFiles          int   `json:"driveFiles"`
	Rooms               int   `json:"rooms"`
	UpcomingEvents      int   `json:"upcomingEvents"`
	ChatMessages24Hours int   `json:"chatMessages24Hours"`
	DriveStorageBytes   int64 `json:"driveStorageBytes"`
	ChatStorageBytes    int64 `json:"chatStorageBytes"`
	TotalStorageBytes   int64 `json:"totalStorageBytes"`
}

type adminDailyMeeting struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type adminRecentMeeting struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Status           string     `json:"status"`
	HostName         string     `json:"hostName"`
	ParticipantCount int        `json:"participantCount"`
	ScheduledAt      *time.Time `json:"scheduledAt"`
	CreatedAt        time.Time  `json:"createdAt"`
}

func (api *API) adminDashboard(writer http.ResponseWriter, request *http.Request, user currentUser) {
	if !api.hasPermission(request.Context(), user, "analytics.read") {
		errorJSON(writer, http.StatusForbidden, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	var meetings adminMeetingSummary
	err := api.database.QueryRowContext(request.Context(), `
		SELECT COUNT(*),COUNT(*) FILTER (WHERE status='SCHEDULED'),
		       COUNT(*) FILTER (WHERE status='ACTIVE'),COUNT(*) FILTER (WHERE status='ENDED')
		FROM meetings WHERE tenant_id=$1`, user.TenantID).
		Scan(&meetings.Total, &meetings.Scheduled, &meetings.Active, &meetings.Ended)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load admin summary")
		return
	}
	var usage adminUsageSummary
	if err = api.database.QueryRowContext(request.Context(), `
		SELECT
		  (SELECT COUNT(*) FROM users WHERE tenant_id=$1 AND status!='DEACTIVATED'),
		  (SELECT COUNT(DISTINCT user_id) FROM meeting_participants WHERE tenant_id=$1 AND user_id IS NOT NULL),
		  (SELECT COUNT(*) FROM meeting_participants WHERE tenant_id=$1 AND status='JOINED'),
		  (SELECT COUNT(*) FROM recordings WHERE tenant_id=$1 AND status='READY'),
		  (SELECT COALESCE(SUM(duration_seconds),0) FROM recordings WHERE tenant_id=$1 AND status='READY'),
		  (SELECT COALESCE(SUM(size_bytes),0) FROM recordings WHERE tenant_id=$1 AND status='READY')`, user.TenantID).
		Scan(&usage.Users, &usage.Participants, &usage.ActiveParticipants, &usage.Recordings, &usage.RecordingDurationSecs, &usage.RecordingStorageBytes); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load usage summary")
		return
	}

	daily, err := api.adminDailyMeetings(request, user)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meeting trend")
		return
	}
	recent, err := api.adminRecentMeetings(request, user)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load recent meetings")
		return
	}
	databaseStats := api.database.Stats()
	var operations adminOperationalSummary
	if err = api.database.QueryRowContext(request.Context(), `SELECT (SELECT COUNT(*) FROM meetings WHERE tenant_id=$1 AND status='ACTIVE'),(SELECT COUNT(*) FROM meeting_participants WHERE tenant_id=$1 AND status='WAITING_ROOM'),(SELECT COUNT(*) FROM meeting_participants WHERE tenant_id=$1 AND status='JOINED'),(SELECT COUNT(*) FROM recordings WHERE tenant_id=$1 AND status='FAILED'),(SELECT COUNT(*) FROM drive_nodes WHERE tenant_id=$1 AND kind='FILE' AND deleted_at IS NULL),(SELECT COUNT(*) FROM workspace_rooms WHERE tenant_id=$1),(SELECT COUNT(*) FROM calendar_events WHERE tenant_id=$1 AND ends_at>=NOW()),(SELECT COUNT(*) FROM chat_messages WHERE tenant_id=$1 AND created_at>=NOW()-INTERVAL '24 hours'),(SELECT COALESCE(SUM(size_bytes),0) FROM drive_nodes WHERE tenant_id=$1 AND kind='FILE' AND deleted_at IS NULL),(SELECT COALESCE(SUM(size_bytes),0) FROM chat_attachments WHERE tenant_id=$1),(SELECT COALESCE(SUM(size_bytes),0) FROM drive_nodes WHERE tenant_id=$1 AND kind='FILE' AND deleted_at IS NULL)+(SELECT COALESCE(SUM(size_bytes),0) FROM chat_attachments WHERE tenant_id=$1)+(SELECT COALESCE(SUM(size_bytes),0) FROM recordings WHERE tenant_id=$1 AND status='READY')`, user.TenantID).Scan(&operations.ActiveRooms, &operations.WaitingParticipants, &operations.JoinedParticipants, &operations.FailedRecordings, &operations.DriveFiles, &operations.Rooms, &operations.UpcomingEvents, &operations.ChatMessages24Hours, &operations.DriveStorageBytes, &operations.ChatStorageBytes, &operations.TotalStorageBytes); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load operational telemetry")
		return
	}
	var errors24Hours int
	if err = api.database.QueryRowContext(request.Context(), `SELECT COUNT(*) FROM error_events WHERE tenant_id=$1 AND created_at>=NOW()-INTERVAL '24 hours'`, user.TenantID).Scan(&errors24Hours); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load error telemetry")
		return
	}
	respondJSON(writer, http.StatusOK, map[string]any{
		"tenant":         map[string]string{"id": user.TenantID, "slug": user.TenantSlug, "name": user.TenantName},
		"meetings":       meetings,
		"usage":          usage,
		"dailyMeetings":  daily,
		"recentMeetings": recent,
		"observability":  map[string]any{"media": operations, "database": map[string]any{"openConnections": databaseStats.OpenConnections, "inUse": databaseStats.InUse, "idle": databaseStats.Idle, "maxOpenConnections": databaseStats.MaxOpenConnections, "waitCount": databaseStats.WaitCount, "waitDurationMs": databaseStats.WaitDuration.Milliseconds()}},
		"health": map[string]any{
			"status": "operational", "api": "ok", "postgres": "ok",
			"release":                 currentRelease(),
			"errors24Hours":           errors24Hours,
			"databaseOpenConnections": databaseStats.OpenConnections,
			"checkedAt":               time.Now().UTC(),
		},
	})
}

func (api *API) adminDailyMeetings(request *http.Request, user currentUser) ([]adminDailyMeeting, error) {
	rows, err := api.database.QueryContext(request.Context(), `
		SELECT day::date::text,COUNT(m.id)
		FROM generate_series(CURRENT_DATE-6,CURRENT_DATE,INTERVAL '1 day') day
		LEFT JOIN meetings m ON m.tenant_id=$1 AND m.created_at>=day AND m.created_at<day+INTERVAL '1 day'
		GROUP BY day ORDER BY day`, user.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]adminDailyMeeting, 0, 7)
	for rows.Next() {
		var item adminDailyMeeting
		if err = rows.Scan(&item.Date, &item.Count); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (api *API) adminRecentMeetings(request *http.Request, user currentUser) ([]adminRecentMeeting, error) {
	rows, err := api.database.QueryContext(request.Context(), `
		SELECT m.id,m.title,m.status,u.display_name,
		       (SELECT COUNT(*) FROM meeting_participants p WHERE p.meeting_id=m.id AND p.tenant_id=m.tenant_id),
		       m.scheduled_at,m.created_at
		FROM meetings m JOIN users u ON u.id=m.host_id AND u.tenant_id=m.tenant_id
		WHERE m.tenant_id=$1 ORDER BY m.created_at DESC LIMIT 8`, user.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]adminRecentMeeting, 0, 8)
	for rows.Next() {
		var item adminRecentMeeting
		if err = rows.Scan(&item.ID, &item.Title, &item.Status, &item.HostName, &item.ParticipantCount, &item.ScheduledAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
