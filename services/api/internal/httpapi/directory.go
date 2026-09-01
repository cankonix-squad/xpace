package httpapi

import (
	"net/http"
	"strings"
	"time"
)

type directoryPerson struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	Timezone    string    `json:"timezone"`
	Locale      string    `json:"locale"`
	Bio         string    `json:"bio"`
	AvatarURL   *string   `json:"avatarUrl"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (api *API) directoryUsers(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	rows, err := api.database.QueryContext(request.Context(), `
		SELECT u.id,u.username,u.display_name,u.email,u.role,
		       COALESCE(p.timezone,'Asia/Jakarta'),COALESCE(p.locale,'en-ID'),COALESCE(p.bio,''),
		       COALESCE(NULLIF(BTRIM(p.avatar_url),''),'')<>'',u.created_at
		FROM users u
		LEFT JOIN user_profiles p ON p.user_id=u.id AND p.tenant_id=u.tenant_id
		WHERE u.tenant_id=$1 AND u.status='ACTIVE'
		ORDER BY u.display_name LIMIT 500`, actor.TenantID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load workspace directory")
		return
	}
	defer rows.Close()
	items := make([]directoryPerson, 0)
	for rows.Next() {
		var item directoryPerson
		var hasAvatar bool
		if err := rows.Scan(&item.ID, &item.Username, &item.DisplayName, &item.Email, &item.Role, &item.Timezone, &item.Locale, &item.Bio, &hasAvatar, &item.CreatedAt); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load workspace directory")
			return
		}
		if hasAvatar {
			item.AvatarURL = stringPointer("/api/v1/directory/users/" + item.ID + "/avatar")
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load workspace directory")
		return
	}
	respondJSON(writer, http.StatusOK, map[string]any{"users": items})
}

func (api *API) directoryUserAvatar(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	userID := strings.TrimSpace(request.PathValue("userID"))
	if userID == "" {
		errorJSON(writer, http.StatusNotFound, "AVATAR_NOT_FOUND", "profile picture is not available")
		return
	}
	api.streamProfileAvatar(writer, request, actor.TenantID, userID)
}
