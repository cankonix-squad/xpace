package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

type alertmanagerWebhookPayload struct {
	Status string `json:"status"`
	Alerts []struct {
		Status       string            `json:"status"`
		Labels       map[string]string `json:"labels"`
		Annotations  map[string]string `json:"annotations"`
		Fingerprint  string            `json:"fingerprint"`
		GeneratorURL string            `json:"generatorURL"`
	} `json:"alerts"`
}

func (api *API) alertmanagerWebhook(writer http.ResponseWriter, request *http.Request) {
	secret := strings.TrimSpace(os.Getenv("ALERTMANAGER_WEBHOOK_SECRET"))
	token := strings.TrimSpace(strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "))
	if len(secret) < 32 || len(token) != len(secret) || subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
		errorJSON(writer, http.StatusUnauthorized, "INVALID_WEBHOOK_TOKEN", "alert webhook authentication failed")
		return
	}
	var payload alertmanagerWebhookPayload
	if decodeJSON(writer, request, &payload) != nil {
		return
	}
	if len(payload.Alerts) == 0 || len(payload.Alerts) > 100 {
		errorJSON(writer, http.StatusBadRequest, "INVALID_ALERT_PAYLOAD", "alert payload must contain between 1 and 100 alerts")
		return
	}
	tenantSlug := strings.TrimSpace(os.Getenv("XSPACE_OPERATIONS_TENANT_SLUG"))
	if tenantSlug == "" {
		tenantSlug = "cankonix"
	}
	var tenantID string
	if err := api.database.QueryRowContext(request.Context(), `SELECT id FROM tenants WHERE slug=$1 AND platform_status='ACTIVE'`, tenantSlug).Scan(&tenantID); err != nil {
		errorJSON(writer, http.StatusServiceUnavailable, "OPERATIONS_TENANT_UNAVAILABLE", "operations tenant is unavailable")
		return
	}
	processed := 0
	for _, alert := range payload.Alerts {
		status := strings.ToLower(strings.TrimSpace(alert.Status))
		if status != "firing" && status != "resolved" {
			errorJSON(writer, http.StatusBadRequest, "INVALID_ALERT_PAYLOAD", "alert status must be firing or resolved")
			return
		}
		if err := api.upsertAlertIncident(request, tenantID, status, alert.Labels, alert.Annotations, alert.Fingerprint, alert.GeneratorURL); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INCIDENT_INGESTION_FAILED", "could not ingest alert incident")
			return
		}
		processed++
	}
	respondJSON(writer, http.StatusOK, map[string]int{"processed": processed})
}

func (api *API) upsertAlertIncident(request *http.Request, tenantID, alertStatus string, labels, annotations map[string]string, fingerprint, generatorURL string) error {
	alertName := cleanAlertText(labels["alertname"], 160)
	if alertName == "" {
		alertName = "Prometheus alert"
	}
	title := cleanAlertText(annotations["summary"], 160)
	if title == "" {
		title = alertName
	}
	description := cleanAlertText(annotations["description"], 4000)
	severity := alertIncidentSeverity(labels["severity"])
	fingerprint = cleanAlertText(fingerprint, 256)
	if fingerprint == "" {
		encoded, _ := json.Marshal(map[string]any{"name": alertName, "labels": labels})
		digest := sha256.Sum256(encoded)
		fingerprint = hex.EncodeToString(digest[:])
	}
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var incidentID, currentStatus string
	err = tx.QueryRowContext(request.Context(), `SELECT id,status FROM incidents WHERE tenant_id=$1 AND source='PROMETHEUS' AND external_key=$2 FOR UPDATE`, tenantID, fingerprint).Scan(&incidentID, &currentStatus)
	created := false
	if err == sql.ErrNoRows {
		created = true
		initialStatus := "OPEN"
		if alertStatus == "resolved" {
			initialStatus = "RESOLVED"
		}
		err = tx.QueryRowContext(request.Context(), `INSERT INTO incidents(tenant_id,title,description,source,severity,status,external_key,resolved_at) VALUES($1,$2,$3,'PROMETHEUS',$4,$5,$6,CASE WHEN $5='RESOLVED' THEN NOW() END) RETURNING id,status`, tenantID, title, description, severity, initialStatus, fingerprint).Scan(&incidentID, &currentStatus)
	} else if err == nil {
		nextStatus := currentStatus
		if alertStatus == "resolved" && currentStatus != "CLOSED" {
			nextStatus = "RESOLVED"
		} else if alertStatus == "firing" && (currentStatus == "RESOLVED" || currentStatus == "CLOSED") {
			nextStatus = "OPEN"
		}
		_, err = tx.ExecContext(request.Context(), `UPDATE incidents SET title=$3,description=$4,severity=$5,status=$6,resolved_at=CASE WHEN $6='RESOLVED' THEN COALESCE(resolved_at,NOW()) WHEN $6='OPEN' THEN NULL ELSE resolved_at END,resolved_by=CASE WHEN $6='OPEN' THEN NULL ELSE resolved_by END,updated_at=NOW() WHERE tenant_id=$1 AND id=$2`, tenantID, incidentID, title, description, severity, nextStatus)
		currentStatus = nextStatus
	}
	if err != nil {
		return err
	}
	eventType := "ALERT_FIRING"
	if alertStatus == "resolved" {
		eventType = "ALERT_RESOLVED"
	}
	metadata := mustJSON(map[string]any{"alertname": alertName, "fingerprint": fingerprint, "generatorUrl": cleanAlertText(generatorURL, 1000), "labels": labels, "created": created, "status": currentStatus})
	if _, err = tx.ExecContext(request.Context(), `INSERT INTO incident_events(tenant_id,incident_id,event_type,note,metadata) VALUES($1,$2,$3,$4,$5)`, tenantID, incidentID, eventType, description, metadata); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return api.writeAuditEvent(request.Context(), request, tenantID, "", "incident.alert."+alertStatus, "incident", incidentID, map[string]any{"alertname": alertName, "fingerprint": fingerprint, "severity": severity})
}

func alertIncidentSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical":
		return "P1"
	case "warning":
		return "P2"
	case "info":
		return "P4"
	default:
		return "P3"
	}
}

func cleanAlertText(value string, maximum int) string {
	value = strings.TrimSpace(value)
	characters := []rune(value)
	if len(characters) > maximum {
		value = string(characters[:maximum])
	}
	return value
}
