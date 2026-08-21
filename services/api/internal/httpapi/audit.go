package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

func (api *API) writeAuditEvent(ctx context.Context, request *http.Request, tenantID, actorID, action, resourceType, resourceID string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	encoded, err := json.Marshal(metadata)
	if err == nil {
		_, err = api.database.ExecContext(ctx, `INSERT INTO audit_events (tenant_id,actor_user_id,action,resource_type,resource_id,ip_address,user_agent,metadata) VALUES ($1,NULLIF($2,'')::uuid,$3,$4,NULLIF($5,''),NULLIF($6,'')::inet,$7,$8::jsonb)`, tenantID, actorID, action, resourceType, resourceID, clientIP(request), request.UserAgent(), encoded)
	}
	if err != nil {
		slog.Error("audit event persistence failed", "action", action, "tenant_id", tenantID, "error", err)
	}
	return err
}
