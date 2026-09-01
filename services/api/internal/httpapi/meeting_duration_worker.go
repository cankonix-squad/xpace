package httpapi

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/livekit/protocol/livekit"
)

const meetingDurationWorkerLockID int64 = 894166327004

type meetingDurationWorkerConfig struct {
	Enabled      bool
	InitialDelay time.Duration
	Interval     time.Duration
}

type expiredMeeting struct {
	ID, TenantID, RoomName, Status string
	LimitMinutes                   int64
}

type meetingRoomCloser func(context.Context, string) error

func loadMeetingDurationWorkerConfig() meetingDurationWorkerConfig {
	config := meetingDurationWorkerConfig{Enabled: true, InitialDelay: 30 * time.Second, Interval: 30 * time.Second}
	if value := strings.TrimSpace(os.Getenv("MEETING_DURATION_WORKER_ENABLED")); strings.EqualFold(value, "false") || value == "0" {
		config.Enabled = false
	}
	if value, err := time.ParseDuration(strings.TrimSpace(os.Getenv("MEETING_DURATION_INITIAL_DELAY"))); err == nil && value >= 5*time.Second && value <= time.Hour {
		config.InitialDelay = value
	}
	if value, err := time.ParseDuration(strings.TrimSpace(os.Getenv("MEETING_DURATION_INTERVAL"))); err == nil && value >= 10*time.Second && value <= 10*time.Minute {
		config.Interval = value
	}
	return config
}

func StartMeetingDurationWorker(ctx context.Context, database *sql.DB, logger *slog.Logger) {
	config := loadMeetingDurationWorkerConfig()
	if !config.Enabled {
		logger.Info("meeting duration worker disabled")
		return
	}
	go func() {
		timer := time.NewTimer(config.InitialDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		for {
			runMeetingDurationEnforcement(ctx, database, logger)
			timer.Reset(config.Interval)
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		}
	}()
	logger.Info("meeting duration worker scheduled", "initial_delay", config.InitialDelay.String(), "interval", config.Interval.String())
}

func runMeetingDurationEnforcement(ctx context.Context, database *sql.DB, logger *slog.Logger) {
	connection, err := database.Conn(ctx)
	if err != nil {
		logger.Error("meeting duration worker could not reserve database connection", "error", err)
		return
	}
	defer connection.Close()
	var locked bool
	if err = connection.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, meetingDurationWorkerLockID).Scan(&locked); err != nil || !locked {
		if err != nil {
			logger.Error("meeting duration worker lock failed", "error", err)
		}
		return
	}
	defer connection.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, meetingDurationWorkerLockID)

	rows, err := database.QueryContext(ctx, `
		SELECT m.id,m.tenant_id,m.room_name,m.status::text,
		  LEAST(
		    COALESCE(e.limit_value,p.max_meeting_duration_minutes,120),
		    COALESCE(c.max_meeting_duration_minutes,e.limit_value,p.max_meeting_duration_minutes,120)
		  ) AS effective_limit
		FROM meetings m
		LEFT JOIN tenant_subscriptions s ON s.tenant_id=m.tenant_id
		LEFT JOIN saas_plans p ON p.key=s.plan_key
		LEFT JOIN tenant_entitlements e ON e.tenant_id=m.tenant_id AND e.entitlement_key='meeting.durationMinutes'
		LEFT JOIN tenant_system_configurations c ON c.tenant_id=m.tenant_id
		WHERE (m.status='ACTIVE' AND m.started_at IS NOT NULL AND
		       m.started_at + LEAST(COALESCE(e.limit_value,p.max_meeting_duration_minutes,120),COALESCE(c.max_meeting_duration_minutes,e.limit_value,p.max_meeting_duration_minutes,120))*INTERVAL '1 minute' <= NOW())
		   OR (m.status='ENDED' AND m.end_reason='PLAN_DURATION_LIMIT' AND m.room_closed_at IS NULL)
		ORDER BY m.started_at NULLS LAST,m.id
		LIMIT 50`)
	if err != nil {
		logger.Error("meeting duration worker could not load expired meetings", "error", err)
		return
	}
	items := make([]expiredMeeting, 0)
	for rows.Next() {
		var item expiredMeeting
		if rows.Scan(&item.ID, &item.TenantID, &item.RoomName, &item.Status, &item.LimitMinutes) == nil {
			items = append(items, item)
		}
	}
	rows.Close()
	for _, item := range items {
		if err = enforceMeetingDuration(ctx, database, item, closeLiveKitRoom); err != nil {
			logger.Error("meeting duration enforcement failed", "meeting_id", item.ID, "tenant_id", item.TenantID, "error", err)
			continue
		}
		logger.Info("meeting ended at plan duration limit", "meeting_id", item.ID, "tenant_id", item.TenantID, "limit_minutes", item.LimitMinutes)
	}
}

func enforceMeetingDuration(ctx context.Context, database *sql.DB, item expiredMeeting, closeRoom meetingRoomCloser) error {
	if item.Status == "ACTIVE" {
		tx, err := database.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		result, err := tx.ExecContext(ctx, `UPDATE meetings SET status='ENDED',ended_at=NOW(),end_reason='PLAN_DURATION_LIMIT',updated_at=NOW() WHERE id=$1 AND tenant_id=$2 AND status='ACTIVE' AND started_at+$3*INTERVAL '1 minute'<=NOW()`, item.ID, item.TenantID, item.LimitMinutes)
		if err != nil {
			return err
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			return nil
		}
		if _, err = tx.ExecContext(ctx, `UPDATE meeting_participants SET status='LEFT',left_at=COALESCE(left_at,NOW()) WHERE meeting_id=$1 AND status IN ('PRE_JOIN','WAITING_ROOM','JOINED','DISCONNECTED')`, item.ID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO audit_events(tenant_id,action,resource_type,resource_id,metadata) VALUES($1,'meeting.auto_ended','meeting',$2,jsonb_build_object('reason','PLAN_DURATION_LIMIT','maxMeetingDurationMinutes',$3::bigint))`, item.TenantID, item.ID, item.LimitMinutes); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	if err := closeRoom(ctx, item.RoomName); err != nil && !roomAlreadyClosed(err) {
		return fmt.Errorf("close LiveKit room: %w", err)
	}
	_, err := database.ExecContext(ctx, `UPDATE meetings SET room_closed_at=COALESCE(room_closed_at,NOW()),updated_at=NOW() WHERE id=$1 AND tenant_id=$2 AND status='ENDED' AND end_reason='PLAN_DURATION_LIMIT'`, item.ID, item.TenantID)
	return err
}

func roomAlreadyClosed(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not found") || strings.Contains(message, "not_found") || strings.Contains(message, "does not exist")
}

func closeLiveKitRoom(ctx context.Context, roomName string) error {
	client, err := liveKitRoomClient()
	if err != nil {
		return err
	}
	_, err = client.DeleteRoom(ctx, &livekit.DeleteRoomRequest{Room: roomName})
	return err
}
