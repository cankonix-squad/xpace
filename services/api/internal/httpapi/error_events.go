package httpapi

import (
	"net/http"
	"os"
	"strings"
)

func (api *API) clientError(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	var input struct {
		Message string `json:"message"`
		Digest  string `json:"digest"`
		Path    string `json:"path"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", err.Error())
		return
	}
	input.Message = strings.TrimSpace(input.Message)
	if input.Message == "" || len(input.Message) > 1000 || len(input.Digest) > 200 || len(input.Path) > 500 {
		errorJSON(writer, 400, "INVALID_INPUT", "invalid error event")
		return
	}
	_, err := api.database.ExecContext(request.Context(), `INSERT INTO error_events (tenant_id,user_id,source,message,digest,path,release,user_agent) VALUES ($1,$2,'WEB',$3,NULLIF($4,''),NULLIF($5,''),$6,NULLIF($7,''))`, actor.TenantID, actor.ID, input.Message, strings.TrimSpace(input.Digest), strings.TrimSpace(input.Path), envOr("XPACE_RELEASE", "development"), request.UserAgent())
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not record error event")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func currentRelease() string {
	if value := strings.TrimSpace(os.Getenv("XPACE_RELEASE")); value != "" {
		return value
	}
	return "development"
}
