package httpapi

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"strings"
	"time"
)

const governanceWorkerLockID int64 = 894166327001

type governanceWorkerConfig struct {
	Enabled      bool
	InitialDelay time.Duration
	Interval     time.Duration
}

func loadGovernanceWorkerConfig() governanceWorkerConfig {
	config := governanceWorkerConfig{Enabled: true, InitialDelay: time.Minute, Interval: 24 * time.Hour}
	if value := strings.TrimSpace(os.Getenv("GOVERNANCE_RETENTION_WORKER_ENABLED")); strings.EqualFold(value, "false") || value == "0" {
		config.Enabled = false
	}
	if value, err := time.ParseDuration(strings.TrimSpace(os.Getenv("GOVERNANCE_RETENTION_INITIAL_DELAY"))); err == nil && value >= 10*time.Second && value <= 24*time.Hour {
		config.InitialDelay = value
	}
	if value, err := time.ParseDuration(strings.TrimSpace(os.Getenv("GOVERNANCE_RETENTION_INTERVAL"))); err == nil && value >= 5*time.Minute && value <= 7*24*time.Hour {
		config.Interval = value
	}
	return config
}

func StartGovernanceRetentionWorker(ctx context.Context, database *sql.DB, logger *slog.Logger) {
	config := loadGovernanceWorkerConfig()
	if !config.Enabled {
		logger.Info("governance retention worker disabled")
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
			runScheduledGovernanceRetention(ctx, database, logger)
			timer.Reset(config.Interval)
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		}
	}()
	logger.Info("governance retention worker scheduled", "initial_delay", config.InitialDelay.String(), "interval", config.Interval.String())
}

func runScheduledGovernanceRetention(ctx context.Context, database *sql.DB, logger *slog.Logger) {
	connection, err := database.Conn(ctx)
	if err != nil {
		logger.Error("governance worker could not reserve database connection", "error", err)
		return
	}
	defer connection.Close()
	var locked bool
	if err = connection.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, governanceWorkerLockID).Scan(&locked); err != nil || !locked {
		if err != nil {
			logger.Error("governance worker lock failed", "error", err)
		}
		return
	}
	defer connection.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, governanceWorkerLockID)

	rows, err := database.QueryContext(ctx, `SELECT id FROM tenants ORDER BY id`)
	if err != nil {
		logger.Error("governance worker could not load tenants", "error", err)
		return
	}
	tenantIDs := make([]string, 0)
	for rows.Next() {
		var tenantID string
		if rows.Scan(&tenantID) == nil {
			tenantIDs = append(tenantIDs, tenantID)
		}
	}
	rows.Close()
	api := &API{database: database}
	for _, tenantID := range tenantIDs {
		result, runErr := api.applyGovernanceRetention(ctx, tenantID)
		if runErr != nil {
			logger.Error("scheduled governance retention failed", "tenant_id", tenantID, "error", runErr)
			continue
		}
		_, auditErr := database.ExecContext(ctx, `INSERT INTO audit_events(tenant_id,action,resource_type,resource_id,metadata) VALUES($1,'governance.retention.scheduled','tenant',$8,jsonb_build_object('chatMessages',$2::bigint,'chatAttachments',$3::bigint,'recordings',$4::bigint,'recordingObjects',$5::bigint,'driveFiles',$6::bigint,'auditEvents',$7::bigint))`, tenantID, result.ChatMessages, result.ChatAttachments, result.Recordings, result.RecordingObjects, result.DriveFiles, result.AuditEvents, tenantID)
		if auditErr != nil {
			logger.Error("scheduled governance audit failed", "tenant_id", tenantID, "error", auditErr)
		}
		logger.Info("scheduled governance retention completed", "tenant_id", tenantID, "chat_messages", result.ChatMessages, "chat_attachments", result.ChatAttachments, "recordings", result.Recordings, "recording_objects", result.RecordingObjects, "drive_files", result.DriveFiles, "audit_events", result.AuditEvents)
	}
}
