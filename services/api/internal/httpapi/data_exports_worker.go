package httpapi

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
)

const dataExportWorkerLockID int64 = 894166327002

type dataExportWorkerConfig struct {
	Enabled      bool
	InitialDelay time.Duration
	Interval     time.Duration
}

type exportJob struct{ ID, TenantID, ExportType string }

type exportCollection struct{ Name, Query string }

func loadDataExportWorkerConfig() dataExportWorkerConfig {
	config := dataExportWorkerConfig{Enabled: true, InitialDelay: 15 * time.Second, Interval: 30 * time.Second}
	if value := strings.TrimSpace(os.Getenv("DATA_EXPORT_WORKER_ENABLED")); strings.EqualFold(value, "false") || value == "0" {
		config.Enabled = false
	}
	if value, err := time.ParseDuration(strings.TrimSpace(os.Getenv("DATA_EXPORT_WORKER_INITIAL_DELAY"))); err == nil && value >= 5*time.Second && value <= time.Hour {
		config.InitialDelay = value
	}
	if value, err := time.ParseDuration(strings.TrimSpace(os.Getenv("DATA_EXPORT_WORKER_INTERVAL"))); err == nil && value >= 10*time.Second && value <= time.Hour {
		config.Interval = value
	}
	return config
}

func StartDataExportWorker(ctx context.Context, database *sql.DB, logger *slog.Logger) {
	config := loadDataExportWorkerConfig()
	if !config.Enabled {
		logger.Info("data export worker disabled")
		return
	}
	go func() {
		timer := time.NewTimer(config.InitialDelay)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				runDataExportWorker(ctx, database, logger)
				timer.Reset(config.Interval)
			}
		}
	}()
	logger.Info("data export worker scheduled", "initial_delay", config.InitialDelay.String(), "interval", config.Interval.String())
}

func runDataExportWorker(ctx context.Context, database *sql.DB, logger *slog.Logger) {
	connection, err := database.Conn(ctx)
	if err != nil {
		logger.Error("data export worker could not reserve database connection", "error", err)
		return
	}
	defer connection.Close()
	var locked bool
	if err = connection.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, dataExportWorkerLockID).Scan(&locked); err != nil || !locked {
		if err != nil {
			logger.Error("data export worker lock failed", "error", err)
		}
		return
	}
	defer connection.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, dataExportWorkerLockID)
	api := &API{database: database}
	api.cleanupExpiredDataExports(ctx, logger)
	for processed := 0; processed < 3; processed++ {
		job, found, claimErr := claimDataExport(ctx, database)
		if claimErr != nil {
			logger.Error("data export claim failed", "error", claimErr)
			return
		}
		if !found {
			return
		}
		if err := api.generateDataExport(ctx, job); err != nil {
			_, _ = database.ExecContext(ctx, `UPDATE data_export_requests SET status='FAILED',error_message=$1,updated_at=NOW() WHERE id=$2 AND tenant_id=$3 AND status='PROCESSING'`, "export generation failed", job.ID, job.TenantID)
			logger.Error("data export generation failed", "export_id", job.ID, "tenant_id", job.TenantID, "error", err)
			continue
		}
		_, auditErr := database.ExecContext(ctx, `INSERT INTO audit_events(tenant_id,action,resource_type,resource_id,metadata) VALUES($1,'governance.export.ready','data_export',$2,jsonb_build_object('exportType',$3::text))`, job.TenantID, job.ID, job.ExportType)
		if auditErr != nil {
			logger.Error("data export ready audit failed", "export_id", job.ID, "error", auditErr)
		}
		logger.Info("data export ready", "export_id", job.ID, "tenant_id", job.TenantID, "export_type", job.ExportType)
	}
}

func claimDataExport(ctx context.Context, database *sql.DB) (exportJob, bool, error) {
	var job exportJob
	err := database.QueryRowContext(ctx, `UPDATE data_export_requests SET status='PROCESSING',updated_at=NOW() WHERE id=(SELECT id FROM data_export_requests WHERE status='APPROVED' ORDER BY reviewed_at,created_at FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING id,tenant_id,export_type`).Scan(&job.ID, &job.TenantID, &job.ExportType)
	if err == sql.ErrNoRows {
		return job, false, nil
	}
	return job, err == nil, err
}

func (api *API) generateDataExport(ctx context.Context, job exportJob) error {
	file, err := os.CreateTemp("", "xpace-export-*.jsonl.gz")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	defer file.Close()
	if err = file.Chmod(0o600); err != nil {
		return err
	}
	hash := sha256.New()
	archive := gzip.NewWriter(io.MultiWriter(file, hash))
	archive.Name = "xpace-" + strings.ToLower(job.ExportType) + "-export.jsonl"
	encoder := json.NewEncoder(archive)
	if err = encoder.Encode(map[string]any{"collection": "manifest", "data": map[string]any{"schemaVersion": 1, "exportId": job.ID, "tenantId": job.TenantID, "exportType": job.ExportType, "generatedAt": time.Now().UTC()}}); err != nil {
		archive.Close()
		return err
	}
	for _, collection := range collectionsForDataExport(job.ExportType) {
		rows, queryErr := api.database.QueryContext(ctx, collection.Query, job.TenantID)
		if queryErr != nil {
			archive.Close()
			return fmt.Errorf("collection %s: %w", collection.Name, queryErr)
		}
		for rows.Next() {
			var raw []byte
			if queryErr = rows.Scan(&raw); queryErr != nil {
				break
			}
			if queryErr = encoder.Encode(map[string]any{"collection": collection.Name, "data": json.RawMessage(raw)}); queryErr != nil {
				break
			}
		}
		if closeErr := rows.Close(); queryErr == nil {
			queryErr = closeErr
		}
		if queryErr != nil {
			archive.Close()
			return fmt.Errorf("collection %s: %w", collection.Name, queryErr)
		}
	}
	if err = archive.Close(); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	client, bucket, err := recordingObjectClient()
	if err != nil {
		return err
	}
	objectKey := fmt.Sprintf("exports/%s/%s.jsonl.gz", job.TenantID, job.ID)
	if _, err = client.PutObject(ctx, bucket, objectKey, file, stat.Size(), minio.PutObjectOptions{ContentType: "application/gzip"}); err != nil {
		return err
	}
	checksum := hex.EncodeToString(hash.Sum(nil))
	result, err := api.database.ExecContext(ctx, `UPDATE data_export_requests SET status='READY',object_key=$1,size_bytes=$2,sha256=$3,expires_at=NOW()+INTERVAL '7 days',error_message=NULL,updated_at=NOW() WHERE id=$4 AND tenant_id=$5 AND status='PROCESSING'`, objectKey, stat.Size(), checksum, job.ID, job.TenantID)
	if err != nil {
		_ = client.RemoveObject(ctx, bucket, objectKey, minio.RemoveObjectOptions{})
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		_ = client.RemoveObject(ctx, bucket, objectKey, minio.RemoveObjectOptions{})
		return fmt.Errorf("export job state changed")
	}
	return nil
}

func collectionsForDataExport(exportType string) []exportCollection {
	directory := []exportCollection{
		{"users", `SELECT (to_jsonb(u)-'password_hash') FROM users u WHERE tenant_id=$1 ORDER BY created_at,id`},
		{"groups", `SELECT to_jsonb(g) FROM groups g WHERE tenant_id=$1 ORDER BY created_at,id`},
		{"group_members", `SELECT to_jsonb(gm) FROM group_members gm WHERE tenant_id=$1 ORDER BY created_at,group_id,user_id`},
	}
	audit := []exportCollection{{"audit_events", `SELECT to_jsonb(a) FROM audit_events a WHERE tenant_id=$1 ORDER BY created_at,id`}}
	if exportType == "DIRECTORY" {
		return directory
	}
	if exportType == "AUDIT" {
		return audit
	}
	full := append([]exportCollection{}, directory...)
	full = append(full,
		exportCollection{"tenant", `SELECT to_jsonb(t) FROM tenants t WHERE id=$1`},
		exportCollection{"meetings", `SELECT to_jsonb(m) FROM meetings m WHERE tenant_id=$1 ORDER BY created_at,id`},
		exportCollection{"meeting_participants", `SELECT to_jsonb(p) FROM meeting_participants p JOIN meetings m ON m.id=p.meeting_id WHERE m.tenant_id=$1 ORDER BY p.created_at,p.id`},
		exportCollection{"recordings", `SELECT (to_jsonb(r)-'object_key') FROM recordings r WHERE tenant_id=$1 ORDER BY created_at,id`},
		exportCollection{"chat_conversations", `SELECT to_jsonb(c) FROM chat_conversations c WHERE tenant_id=$1 ORDER BY created_at,id`},
		exportCollection{"chat_members", `SELECT to_jsonb(cm) FROM chat_members cm WHERE tenant_id=$1 ORDER BY joined_at,conversation_id,user_id`},
		exportCollection{"chat_messages", `SELECT to_jsonb(m) FROM chat_messages m WHERE tenant_id=$1 ORDER BY created_at,id`},
		exportCollection{"chat_attachments", `SELECT (to_jsonb(a)-'object_key') FROM chat_attachments a WHERE tenant_id=$1 ORDER BY created_at,id`},
		exportCollection{"calendar_events", `SELECT to_jsonb(e) FROM calendar_events e WHERE tenant_id=$1 ORDER BY created_at,id`},
		exportCollection{"calendar_attendees", `SELECT to_jsonb(a) FROM calendar_event_attendees a WHERE tenant_id=$1 ORDER BY created_at,event_id,user_id`},
		exportCollection{"workspace_rooms", `SELECT to_jsonb(r) FROM workspace_rooms r WHERE tenant_id=$1 ORDER BY created_at,id`},
		exportCollection{"workspace_room_members", `SELECT to_jsonb(m) FROM workspace_room_members m WHERE tenant_id=$1 ORDER BY joined_at,room_id,user_id`},
		exportCollection{"drive_nodes", `SELECT (to_jsonb(n)-'object_key') FROM drive_nodes n WHERE tenant_id=$1 ORDER BY created_at,id`},
		exportCollection{"drive_shares", `SELECT to_jsonb(s) FROM drive_shares s WHERE tenant_id=$1 ORDER BY created_at,node_id,user_id`},
		exportCollection{"governance_policy", `SELECT to_jsonb(p) FROM tenant_governance_policies p WHERE tenant_id=$1`},
		exportCollection{"legal_holds", `SELECT to_jsonb(h) FROM legal_holds h WHERE tenant_id=$1 ORDER BY created_at,id`},
		exportCollection{"legal_hold_resources", `SELECT to_jsonb(r) FROM legal_hold_resources r WHERE tenant_id=$1 ORDER BY added_at,hold_id,resource_id`},
	)
	return append(full, audit...)
}

func (api *API) cleanupExpiredDataExports(ctx context.Context, logger *slog.Logger) {
	client, bucket, err := recordingObjectClient()
	if err != nil {
		return
	}
	rows, err := api.database.QueryContext(ctx, `SELECT id,tenant_id,object_key FROM data_export_requests WHERE status='READY' AND expires_at<=NOW() ORDER BY expires_at LIMIT 25`)
	if err != nil {
		return
	}
	type expiredExport struct{ id, tenantID, key string }
	items := make([]expiredExport, 0)
	for rows.Next() {
		var item expiredExport
		if rows.Scan(&item.id, &item.tenantID, &item.key) == nil {
			items = append(items, item)
		}
	}
	rows.Close()
	for _, item := range items {
		if client.RemoveObject(ctx, bucket, item.key, minio.RemoveObjectOptions{}) != nil {
			continue
		}
		_, updateErr := api.database.ExecContext(ctx, `UPDATE data_export_requests SET status='EXPIRED',updated_at=NOW() WHERE id=$1 AND tenant_id=$2 AND status='READY' AND expires_at<=NOW()`, item.id, item.tenantID)
		if updateErr != nil {
			logger.Error("data export expiration update failed", "export_id", item.id, "error", updateErr)
		}
	}
}
