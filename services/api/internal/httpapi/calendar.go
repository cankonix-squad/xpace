package httpapi

import (
	"net/http"
	"strings"
	"time"
)

type calendarEvent struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	Timezone         string    `json:"timezone"`
	StartsAt         time.Time `json:"startsAt"`
	EndsAt           time.Time `json:"endsAt"`
	RecurrenceRule   string    `json:"recurrenceRule,omitempty"`
	ReminderMinutes  int       `json:"reminderMinutes"`
	OrganizerID      string    `json:"organizerId"`
	OrganizerName    string    `json:"organizerName"`
	AttendeeStatus   string    `json:"attendeeStatus"`
	AttendeeCount    int       `json:"attendeeCount"`
	MeetingID        *string   `json:"meetingId,omitempty"`
	MeetingJoinCode  string    `json:"meetingJoinCode,omitempty"`
	MeetingStatus    string    `json:"meetingStatus,omitempty"`
	ParticipantCount int       `json:"participantCount"`
}

func (api *API) calendarEvents(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	switch request.Method {
	case http.MethodGet:
		rows, err := api.database.QueryContext(request.Context(), `SELECT e.id,e.title,e.description,e.timezone,e.starts_at,e.ends_at,COALESCE(e.recurrence_rule,''),e.reminder_minutes,e.organizer_id,u.display_name,COALESCE(a.status,'ACCEPTED'),(SELECT COUNT(*) FROM calendar_event_attendees x WHERE x.event_id=e.id AND x.tenant_id=e.tenant_id),e.meeting_id,COALESCE(m.join_code,''),COALESCE(m.status::text,''),(SELECT COUNT(*) FROM meeting_participants p WHERE p.meeting_id=e.meeting_id AND p.tenant_id=e.tenant_id) FROM calendar_events e JOIN users u ON u.id=e.organizer_id AND u.tenant_id=e.tenant_id LEFT JOIN calendar_event_attendees a ON a.event_id=e.id AND a.tenant_id=e.tenant_id AND a.user_id=$2 LEFT JOIN meetings m ON m.id=e.meeting_id AND m.tenant_id=e.tenant_id WHERE e.tenant_id=$1 AND (e.organizer_id=$2 OR a.user_id=$2) ORDER BY e.starts_at LIMIT 200`, actor.TenantID, actor.ID)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load calendar")
			return
		}
		defer rows.Close()
		items := make([]calendarEvent, 0)
		for rows.Next() {
			var item calendarEvent
			if err := rows.Scan(&item.ID, &item.Title, &item.Description, &item.Timezone, &item.StartsAt, &item.EndsAt, &item.RecurrenceRule, &item.ReminderMinutes, &item.OrganizerID, &item.OrganizerName, &item.AttendeeStatus, &item.AttendeeCount, &item.MeetingID, &item.MeetingJoinCode, &item.MeetingStatus, &item.ParticipantCount); err != nil {
				errorJSON(writer, 500, "INTERNAL_ERROR", "could not load calendar")
				return
			}
			items = append(items, item)
		}
		respondJSON(writer, 200, map[string]any{"events": items})
	case http.MethodPost:
		var input struct {
			Title, Description, Timezone, RecurrenceRule string
			StartsAt, EndsAt                             time.Time
			ReminderMinutes                              int
			AttendeeIDs                                  []string
			CreateMeeting                                bool
		}
		if err := decodeJSON(writer, request, &input); err != nil {
			errorJSON(writer, 400, "INVALID_INPUT", err.Error())
			return
		}
		input.Title, input.Description, input.Timezone = strings.TrimSpace(input.Title), strings.TrimSpace(input.Description), strings.TrimSpace(input.Timezone)
		input.RecurrenceRule = strings.ToUpper(strings.TrimSpace(input.RecurrenceRule))
		if len(input.Title) < 3 || len(input.Title) > 160 || !input.EndsAt.After(input.StartsAt) {
			errorJSON(writer, 400, "INVALID_INPUT", "title and event time range are invalid")
			return
		}
		if input.Timezone == "" {
			input.Timezone = "Asia/Jakarta"
		}
		if _, err := time.LoadLocation(input.Timezone); err != nil {
			errorJSON(writer, 400, "INVALID_INPUT", "timezone is invalid")
			return
		}
		if input.RecurrenceRule != "" && input.RecurrenceRule != "DAILY" && input.RecurrenceRule != "WEEKLY" && input.RecurrenceRule != "MONTHLY" {
			errorJSON(writer, 400, "INVALID_INPUT", "recurrenceRule must be DAILY, WEEKLY, or MONTHLY")
			return
		}
		if input.ReminderMinutes < 0 || input.ReminderMinutes > 10080 {
			errorJSON(writer, 400, "INVALID_INPUT", "reminderMinutes is invalid")
			return
		}
		if input.CreateMeeting {
			if err := api.enforceTenantQuota(request.Context(), actor.TenantID, "meetings", 1); err != nil {
				if !respondEntitlementError(writer, err) {
					errorJSON(writer, 500, "INTERNAL_ERROR", "could not verify workspace quota")
				}
				return
			}
		}
		tx, err := api.database.BeginTx(request.Context(), nil)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not create event")
			return
		}
		defer tx.Rollback()
		var meetingID *string
		if input.CreateMeeting {
			var id string
			for attempt := 0; attempt < 3; attempt++ {
				code, _ := meetingCode()
				token, _ := randomToken(12)
				err = tx.QueryRowContext(request.Context(), `INSERT INTO meetings (tenant_id,host_id,room_name,join_code,title,scheduled_at,status,waiting_room_enabled) VALUES ($1,$2,$3,$4,$5,$6,'SCHEDULED',TRUE) RETURNING id`, actor.TenantID, actor.ID, "xpace-"+strings.ToLower(token), code, input.Title, input.StartsAt).Scan(&id)
				if err == nil {
					break
				}
			}
			if err != nil {
				errorJSON(writer, 500, "INTERNAL_ERROR", "could not create linked meeting")
				return
			}
			meetingID = &id
		}
		var event calendarEvent
		err = tx.QueryRowContext(request.Context(), `INSERT INTO calendar_events (tenant_id,organizer_id,meeting_id,title,description,timezone,starts_at,ends_at,recurrence_rule,reminder_minutes) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),$10) RETURNING id,title,description,timezone,starts_at,ends_at,COALESCE(recurrence_rule,''),reminder_minutes,organizer_id,meeting_id`, actor.TenantID, actor.ID, meetingID, input.Title, input.Description, input.Timezone, input.StartsAt, input.EndsAt, input.RecurrenceRule, input.ReminderMinutes).Scan(&event.ID, &event.Title, &event.Description, &event.Timezone, &event.StartsAt, &event.EndsAt, &event.RecurrenceRule, &event.ReminderMinutes, &event.OrganizerID, &event.MeetingID)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not create event")
			return
		}
		members := uniqueIDs(append(input.AttendeeIDs, actor.ID))
		for _, id := range members {
			status := "INVITED"
			if id == actor.ID {
				status = "ACCEPTED"
			}
			result, insertErr := tx.ExecContext(request.Context(), `INSERT INTO calendar_event_attendees (event_id,tenant_id,user_id,status,responded_at) SELECT $1,$2,id,$4,CASE WHEN $4='ACCEPTED' THEN NOW() END FROM users WHERE id=$3 AND tenant_id=$2 AND status='ACTIVE'`, event.ID, actor.TenantID, id, status)
			if insertErr != nil {
				errorJSON(writer, 500, "INTERNAL_ERROR", "could not invite attendee")
				return
			}
			if count, _ := result.RowsAffected(); count > 0 {
				event.AttendeeCount++
			}
		}
		if err = tx.Commit(); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not create event")
			return
		}
		event.OrganizerName = actor.DisplayName
		event.AttendeeStatus = "ACCEPTED"
		_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "calendar.event.created", "calendar_event", event.ID, map[string]any{"recurrence": event.RecurrenceRule, "meetingLinked": meetingID != nil})
		respondJSON(writer, 201, map[string]any{"event": event})
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *API) calendarResponse(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	var input struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", err.Error())
		return
	}
	input.Status = strings.ToUpper(strings.TrimSpace(input.Status))
	if input.Status != "ACCEPTED" && input.Status != "DECLINED" {
		errorJSON(writer, 400, "INVALID_INPUT", "status must be ACCEPTED or DECLINED")
		return
	}
	result, err := api.database.ExecContext(request.Context(), `UPDATE calendar_event_attendees SET status=$1,responded_at=NOW() WHERE event_id=$2 AND tenant_id=$3 AND user_id=$4`, input.Status, request.PathValue("eventID"), actor.TenantID, actor.ID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not respond to invitation")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		errorJSON(writer, 404, "NOT_FOUND", "calendar invitation not found")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}
