package httpapi

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/minio/minio-go/v7"
)

const maxProfileAvatar = 2 << 20

type profileResponse struct {
	UserID      string  `json:"userId"`
	TenantID    string  `json:"tenantId"`
	DisplayName string  `json:"displayName"`
	Email       string  `json:"email"`
	Username    string  `json:"username"`
	Role        string  `json:"role"`
	Timezone    string  `json:"timezone"`
	Locale      string  `json:"locale"`
	Bio         string  `json:"bio"`
	AvatarURL   *string `json:"avatarUrl"`
}

func (api *API) profile(writer http.ResponseWriter, request *http.Request, user currentUser) {
	if request.Method == http.MethodPatch {
		api.updateProfile(writer, request, user)
		return
	}
	api.getProfile(writer, request, user)
}

func (api *API) getProfile(writer http.ResponseWriter, request *http.Request, user currentUser) {
	profile, err := api.loadProfile(request, user)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load profile")
		return
	}
	respondJSON(writer, http.StatusOK, map[string]any{"profile": profile})
}

func (api *API) loadProfile(request *http.Request, user currentUser) (profileResponse, error) {
	var profile profileResponse
	var avatarObjectKey *string
	err := api.database.QueryRowContext(request.Context(), `
		SELECT u.id,u.tenant_id,u.display_name,u.email,u.username,u.role,
		       COALESCE(p.timezone,'Asia/Jakarta'),COALESCE(p.locale,'en-ID'),
		       COALESCE(p.bio,''),p.avatar_url
		FROM users u
		LEFT JOIN user_profiles p ON p.user_id=u.id AND p.tenant_id=u.tenant_id
		WHERE u.id=$1 AND u.tenant_id=$2`, user.ID, user.TenantID).
		Scan(&profile.UserID, &profile.TenantID, &profile.DisplayName, &profile.Email,
			&profile.Username, &profile.Role, &profile.Timezone, &profile.Locale,
			&profile.Bio, &avatarObjectKey)
	if err == nil && avatarObjectKey != nil && strings.TrimSpace(*avatarObjectKey) != "" {
		profile.AvatarURL = stringPointer("/api/v1/profile/avatar?v=" + fmt.Sprint(time.Now().Unix()))
	}
	return profile, err
}

func (api *API) profileAvatar(writer http.ResponseWriter, request *http.Request, user currentUser) {
	switch request.Method {
	case http.MethodGet:
		api.getProfileAvatar(writer, request, user)
	case http.MethodPut:
		api.updateProfileAvatar(writer, request, user)
	case http.MethodDelete:
		api.deleteProfileAvatar(writer, request, user)
	}
}

func (api *API) getProfileAvatar(writer http.ResponseWriter, request *http.Request, user currentUser) {
	api.streamProfileAvatar(writer, request, user.TenantID, user.ID)
}

func (api *API) streamProfileAvatar(writer http.ResponseWriter, request *http.Request, tenantID, userID string) {
	var objectKey string
	if err := api.database.QueryRowContext(request.Context(), `SELECT avatar_url FROM user_profiles WHERE user_id=$1 AND tenant_id=$2 AND avatar_url IS NOT NULL`, userID, tenantID).Scan(&objectKey); err != nil {
		errorJSON(writer, http.StatusNotFound, "AVATAR_NOT_FOUND", "profile picture is not available")
		return
	}
	client, bucket, err := recordingObjectClient()
	if err != nil {
		errorJSON(writer, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "profile storage is unavailable")
		return
	}
	object, err := client.GetObject(request.Context(), bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		errorJSON(writer, http.StatusNotFound, "AVATAR_NOT_FOUND", "profile picture is not available")
		return
	}
	defer object.Close()
	stat, err := object.Stat()
	if err != nil {
		errorJSON(writer, http.StatusNotFound, "AVATAR_NOT_FOUND", "profile picture is not available")
		return
	}
	writer.Header().Set("Content-Type", stat.ContentType)
	writer.Header().Set("Content-Length", fmt.Sprint(stat.Size))
	writer.Header().Set("Cache-Control", "private, max-age=300")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(writer, object)
}

func (api *API) updateProfileAvatar(writer http.ResponseWriter, request *http.Request, user currentUser) {
	if request.ContentLength > maxProfileAvatar+(256<<10) {
		errorJSON(writer, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "profile picture must be 2 MB or smaller")
		return
	}
	if err := request.ParseMultipartForm(maxProfileAvatar); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", "invalid profile picture upload")
		return
	}
	file, header, err := request.FormFile("file")
	if err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", "profile picture is required")
		return
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > maxProfileAvatar {
		errorJSON(writer, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "profile picture must be 2 MB or smaller")
		return
	}
	buffer := make([]byte, 512)
	read, _ := file.Read(buffer)
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", "could not read profile picture")
		return
	}
	contentType := http.DetectContentType(buffer[:read])
	extensions := map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}
	extension, allowed := extensions[contentType]
	if !allowed {
		errorJSON(writer, http.StatusBadRequest, "INVALID_FILE_TYPE", "profile picture must be JPEG, PNG, or WebP")
		return
	}
	client, bucket, err := recordingObjectClient()
	if err != nil {
		errorJSON(writer, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "profile storage is unavailable")
		return
	}
	objectKey := fmt.Sprintf("profiles/%s/%s.%s", user.TenantID, user.ID, extension)
	if _, err = client.PutObject(request.Context(), bucket, objectKey, file, header.Size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		errorJSON(writer, http.StatusBadGateway, "STORAGE_ERROR", "could not store profile picture")
		return
	}
	var previousObjectKey *string
	_ = api.database.QueryRowContext(request.Context(), `SELECT avatar_url FROM user_profiles WHERE user_id=$1 AND tenant_id=$2`, user.ID, user.TenantID).Scan(&previousObjectKey)
	_, err = api.database.ExecContext(request.Context(), `INSERT INTO user_profiles(user_id,tenant_id,avatar_url) VALUES($1,$2,$3) ON CONFLICT(user_id) DO UPDATE SET avatar_url=EXCLUDED.avatar_url,updated_at=NOW() WHERE user_profiles.tenant_id=EXCLUDED.tenant_id`, user.ID, user.TenantID, objectKey)
	if err != nil {
		_ = client.RemoveObject(request.Context(), bucket, objectKey, minio.RemoveObjectOptions{})
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not save profile picture")
		return
	}
	if previousObjectKey != nil && *previousObjectKey != objectKey {
		_ = client.RemoveObject(request.Context(), bucket, *previousObjectKey, minio.RemoveObjectOptions{})
	}
	_ = api.writeAuditEvent(request.Context(), request, user.TenantID, user.ID, "profile.avatar.update", "user", user.ID, map[string]any{"contentType": contentType, "sizeBytes": header.Size})
	profile, err := api.loadProfile(request, user)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "profile picture updated but profile could not be loaded")
		return
	}
	respondJSON(writer, http.StatusOK, map[string]any{"profile": profile})
}

func (api *API) deleteProfileAvatar(writer http.ResponseWriter, request *http.Request, user currentUser) {
	var objectKey string
	err := api.database.QueryRowContext(request.Context(), `SELECT avatar_url FROM user_profiles WHERE user_id=$1 AND tenant_id=$2 AND avatar_url IS NOT NULL`, user.ID, user.TenantID).Scan(&objectKey)
	if err != nil && err != sql.ErrNoRows {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not remove profile picture")
		return
	}
	if objectKey != "" {
		if _, err = api.database.ExecContext(request.Context(), `UPDATE user_profiles SET avatar_url=NULL,updated_at=NOW() WHERE user_id=$1 AND tenant_id=$2`, user.ID, user.TenantID); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not remove profile picture")
			return
		}
	}
	if objectKey != "" {
		if client, bucket, storageErr := recordingObjectClient(); storageErr == nil {
			_ = client.RemoveObject(request.Context(), bucket, objectKey, minio.RemoveObjectOptions{})
		}
	}
	_ = api.writeAuditEvent(request.Context(), request, user.TenantID, user.ID, "profile.avatar.delete", "user", user.ID, nil)
	writer.WriteHeader(http.StatusNoContent)
}

func stringPointer(value string) *string { return &value }

func (api *API) updateProfile(writer http.ResponseWriter, request *http.Request, user currentUser) {
	var input struct {
		DisplayName string `json:"displayName"`
		Timezone    string `json:"timezone"`
		Locale      string `json:"locale"`
		Bio         string `json:"bio"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Timezone = strings.TrimSpace(input.Timezone)
	input.Locale = strings.TrimSpace(input.Locale)
	input.Bio = strings.TrimSpace(input.Bio)
	if utf8.RuneCountInString(input.DisplayName) < 2 || utf8.RuneCountInString(input.DisplayName) > 80 {
		errorJSON(writer, http.StatusBadRequest, "INVALID_DISPLAY_NAME", "displayName must be between 2 and 80 characters")
		return
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_TIMEZONE", "timezone must be a valid IANA timezone")
		return
	}
	if !validLocale(input.Locale) {
		errorJSON(writer, http.StatusBadRequest, "INVALID_LOCALE", "locale must use a language or language-region format")
		return
	}
	if utf8.RuneCountInString(input.Bio) > 280 {
		errorJSON(writer, http.StatusBadRequest, "INVALID_BIO", "bio must not exceed 280 characters")
		return
	}

	tx, err := api.database.BeginTx(request.Context(), &sql.TxOptions{})
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update profile")
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(request.Context(), `UPDATE users SET display_name=$1,updated_at=NOW() WHERE id=$2 AND tenant_id=$3`, input.DisplayName, user.ID, user.TenantID); err == nil {
		_, err = tx.ExecContext(request.Context(), `INSERT INTO user_profiles (user_id,tenant_id,timezone,locale,bio) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (user_id) DO UPDATE SET timezone=EXCLUDED.timezone,locale=EXCLUDED.locale,bio=EXCLUDED.bio,updated_at=NOW() WHERE user_profiles.tenant_id=EXCLUDED.tenant_id`, user.ID, user.TenantID, input.Timezone, input.Locale, input.Bio)
	}
	if err != nil || tx.Commit() != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update profile")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, user.TenantID, user.ID, "profile.update", "user", user.ID, nil)
	profile, err := api.loadProfile(request, user)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "profile updated but could not be reloaded")
		return
	}
	respondJSON(writer, http.StatusOK, map[string]any{"profile": profile})
}

func validLocale(locale string) bool {
	if len(locale) < 2 || len(locale) > 16 {
		return false
	}
	for index, character := range locale {
		if character == '-' && index > 0 && index < len(locale)-1 {
			continue
		}
		if character < 'A' || character > 'Z' && character < 'a' || character > 'z' {
			return false
		}
	}
	return true
}
