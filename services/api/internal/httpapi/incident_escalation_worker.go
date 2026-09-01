package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"time"
)

const incidentEscalationWorkerLockID int64 = 894166327043

type incidentEscalationWorkerConfig struct {
	Enabled                bool
	InitialDelay, Interval time.Duration
}
type escalatedIncident struct{ ID, TenantID, Title, Severity string }

func loadIncidentEscalationWorkerConfig() incidentEscalationWorkerConfig {
	c := incidentEscalationWorkerConfig{true, time.Minute, time.Minute}
	if value := strings.TrimSpace(os.Getenv("INCIDENT_ESCALATION_WORKER_ENABLED")); value == "0" || strings.EqualFold(value, "false") {
		c.Enabled = false
	}
	if value, err := time.ParseDuration(strings.TrimSpace(os.Getenv("INCIDENT_ESCALATION_INITIAL_DELAY"))); err == nil && value >= 5*time.Second && value <= time.Hour {
		c.InitialDelay = value
	}
	if value, err := time.ParseDuration(strings.TrimSpace(os.Getenv("INCIDENT_ESCALATION_INTERVAL"))); err == nil && value >= 10*time.Second && value <= time.Hour {
		c.Interval = value
	}
	return c
}

func StartIncidentEscalationWorker(ctx context.Context, database *sql.DB, logger *slog.Logger) {
	c := loadIncidentEscalationWorkerConfig()
	if !c.Enabled {
		logger.Info("incident escalation worker disabled")
		return
	}
	go func() {
		timer := time.NewTimer(c.InitialDelay)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				runIncidentEscalation(ctx, database, logger)
				timer.Reset(c.Interval)
			}
		}
	}()
	logger.Info("incident escalation worker scheduled", "initial_delay", c.InitialDelay.String(), "interval", c.Interval.String())
}

func runIncidentEscalation(ctx context.Context, database *sql.DB, logger *slog.Logger) {
	connection, err := database.Conn(ctx)
	if err != nil {
		return
	}
	defer connection.Close()
	var locked bool
	if connection.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, incidentEscalationWorkerLockID).Scan(&locked) != nil || !locked {
		return
	}
	defer connection.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, incidentEscalationWorkerLockID)
	rows, err := connection.QueryContext(ctx, `SELECT id,tenant_id,title,severity FROM incidents WHERE escalation_level=0 AND status='OPEN' AND acknowledged_at IS NULL AND created_at<=NOW()-CASE severity WHEN 'P1' THEN INTERVAL '5 minutes' WHEN 'P2' THEN INTERVAL '15 minutes' WHEN 'P3' THEN INTERVAL '1 hour' ELSE INTERVAL '4 hours' END ORDER BY created_at ASC LIMIT 100`)
	if err != nil {
		logger.Error("incident escalation query failed", "error", err)
		return
	}
	items := []escalatedIncident{}
	for rows.Next() {
		var item escalatedIncident
		if rows.Scan(&item.ID, &item.TenantID, &item.Title, &item.Severity) == nil {
			items = append(items, item)
		}
	}
	rows.Close()
	escalated := 0
	for _, item := range items {
		changed, err := escalateIncident(ctx, connection, item)
		if err != nil {
			logger.Error("incident escalation failed", "incident_id", item.ID, "error", err)
			continue
		}
		if changed {
			escalated++
		}
	}
	if escalated > 0 {
		logger.Warn("incidents escalated", "count", escalated)
	}
}

func escalateIncident(ctx context.Context, connection *sql.Conn, item escalatedIncident) (bool, error) {
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `UPDATE incidents SET escalation_level=1,last_escalated_at=NOW(),updated_at=NOW() WHERE id=$1 AND tenant_id=$2 AND escalation_level=0 AND status='OPEN' AND acknowledged_at IS NULL`, item.ID, item.TenantID)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 0 {
		return false, err
	}

	payload := map[string]any{"incidentId": item.ID, "title": item.Title, "severity": item.Severity, "status": "OPEN"}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO incident_events(tenant_id,incident_id,event_type,note,metadata) VALUES($1,$2,'ESCALATED','Acknowledgement SLA exceeded; workspace administrators notified.',$3)`, item.TenantID, item.ID, encoded); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO notifications(tenant_id,recipient_id,type,payload) SELECT $1,id,'INCIDENT_ESCALATION',$2 FROM users WHERE tenant_id=$1 AND status='ACTIVE' AND deleted_at IS NULL AND role IN ('TENANT_ADMIN','SUPER_ADMIN')`, item.TenantID, encoded); err != nil {
		return false, err
	}
	if err = queueWorkspaceAdminNotice(ctx, tx, item.TenantID, "INCIDENT_NOTICE", "incident-escalation:"+item.ID, map[string]any{"publicUrl": envOr("XPACE_PUBLIC_URL", ""), "title": item.Title, "severity": item.Severity, "message": "The incident acknowledgement SLA was exceeded. Review, assign, and acknowledge the case immediately."}); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_events(tenant_id,action,resource_type,resource_id,metadata) VALUES($1,'incident.escalate','incident',$2,$3)`, item.TenantID, item.ID, encoded); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
