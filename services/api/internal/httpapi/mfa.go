package httpapi

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func (api *API) mfaSettings(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	switch request.Method {
	case http.MethodGet:
		var enabled bool
		var enrolled bool
		err := api.database.QueryRowContext(request.Context(), `SELECT TRUE,enabled FROM user_mfa WHERE user_id=$1 AND tenant_id=$2`, actor.ID, actor.TenantID).Scan(&enrolled, &enabled)
		if err != nil && err != sql.ErrNoRows {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load MFA settings")
			return
		}
		respondJSON(writer, 200, map[string]any{"enabled": enabled, "enrolled": enrolled, "recommended": actor.Role.isWorkspaceAdmin()})
	case http.MethodPost:
		var enabled bool
		_ = api.database.QueryRowContext(request.Context(), `SELECT enabled FROM user_mfa WHERE user_id=$1 AND tenant_id=$2`, actor.ID, actor.TenantID).Scan(&enabled)
		if enabled {
			errorJSON(writer, 409, "MFA_ALREADY_ENABLED", "disable MFA before creating a new authenticator secret")
			return
		}
		secretBytes := make([]byte, 20)
		if _, err := rand.Read(secretBytes); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not create MFA secret")
			return
		}
		secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secretBytes)
		encrypted, err := encryptMFASecret(secret)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not protect MFA secret")
			return
		}
		codes := make([]string, 8)
		hashes := make([]string, 8)
		for index := range codes {
			token, _ := randomToken(9)
			codes[index] = strings.ToUpper(strings.ReplaceAll(token[:12], "-", ""))
			sum := sha256.Sum256([]byte(codes[index]))
			hashes[index] = hex.EncodeToString(sum[:])
		}
		encoded, _ := json.Marshal(hashes)
		_, err = api.database.ExecContext(request.Context(), `INSERT INTO user_mfa(user_id,tenant_id,secret_encrypted,recovery_hashes,enabled,confirmed_at,updated_at) VALUES($1,$2,$3,$4,FALSE,NULL,NOW()) ON CONFLICT(user_id) DO UPDATE SET secret_encrypted=EXCLUDED.secret_encrypted,recovery_hashes=EXCLUDED.recovery_hashes,enabled=FALSE,confirmed_at=NULL,updated_at=NOW()`, actor.ID, actor.TenantID, encrypted, encoded)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not start MFA enrollment")
			return
		}
		issuer := "Xspace"
		label := url.PathEscape(actor.TenantSlug + ":" + actor.Username)
		uri := fmt.Sprintf("otpauth://totp/%s?secret=%s&issuer=%s&algorithm=SHA1&digits=6&period=30", label, secret, url.QueryEscape(issuer))
		respondJSON(writer, 201, map[string]any{"secret": secret, "otpauthUrl": uri, "recoveryCodes": codes})
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *API) mfaDisable(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	var input struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", err.Error())
		return
	}
	var encrypted string
	var recoveryJSON []byte
	if err := api.database.QueryRowContext(request.Context(), `SELECT secret_encrypted,recovery_hashes FROM user_mfa WHERE user_id=$1 AND tenant_id=$2 AND enabled=TRUE`, actor.ID, actor.TenantID).Scan(&encrypted, &recoveryJSON); err != nil {
		errorJSON(writer, 404, "MFA_NOT_ENABLED", "MFA is not enabled")
		return
	}
	if valid, _ := verifyMFA(encrypted, recoveryJSON, input.Code); !valid {
		errorJSON(writer, 401, "INVALID_MFA_CODE", "verification code is invalid")
		return
	}
	if _, err := api.database.ExecContext(request.Context(), `DELETE FROM user_mfa WHERE user_id=$1 AND tenant_id=$2`, actor.ID, actor.TenantID); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not disable MFA")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "auth.mfa.disabled", "user", actor.ID, nil)
	if err := queueTransactionalEmail(request.Context(), api.database, actor.TenantID, actor.Email, "SECURITY_NOTICE", "", "security:mfa-disabled:"+actor.ID+":"+time.Now().UTC().Format("200601021504"), map[string]any{"event": "MFA disabled", "message": "Multi-factor authentication was disabled for your Xspace account."}); err != nil {
		slog.Error("could not queue MFA disabled notice", "user_id", actor.ID, "error", err)
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (api *API) mfaConfirm(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	var input struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", err.Error())
		return
	}
	var encrypted string
	if err := api.database.QueryRowContext(request.Context(), `SELECT secret_encrypted FROM user_mfa WHERE user_id=$1 AND tenant_id=$2`, actor.ID, actor.TenantID).Scan(&encrypted); err != nil {
		errorJSON(writer, 404, "MFA_NOT_ENROLLED", "start MFA enrollment first")
		return
	}
	secret, err := decryptMFASecret(encrypted)
	if err != nil || !validTOTP(secret, input.Code, time.Now()) {
		errorJSON(writer, 401, "INVALID_MFA_CODE", "verification code is invalid")
		return
	}
	_, err = api.database.ExecContext(request.Context(), `UPDATE user_mfa SET enabled=TRUE,confirmed_at=NOW(),updated_at=NOW() WHERE user_id=$1 AND tenant_id=$2`, actor.ID, actor.TenantID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not enable MFA")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "auth.mfa.enabled", "user", actor.ID, nil)
	if err := queueTransactionalEmail(request.Context(), api.database, actor.TenantID, actor.Email, "SECURITY_NOTICE", "", "security:mfa-enabled:"+actor.ID+":"+time.Now().UTC().Format("200601021504"), map[string]any{"event": "MFA enabled", "message": "Multi-factor authentication was enabled for your Xspace account."}); err != nil {
		slog.Error("could not queue MFA enabled notice", "user_id", actor.ID, "error", err)
	}
	writer.WriteHeader(204)
}

func verifyMFA(secretEncrypted string, recoveryJSON []byte, code string) (bool, []string) {
	secret, err := decryptMFASecret(secretEncrypted)
	if err == nil && validTOTP(secret, code, time.Now()) {
		var hashes []string
		_ = json.Unmarshal(recoveryJSON, &hashes)
		return true, hashes
	}
	var hashes []string
	_ = json.Unmarshal(recoveryJSON, &hashes)
	sum := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(code))))
	candidate := hex.EncodeToString(sum[:])
	for index, value := range hashes {
		if subtle.ConstantTimeCompare([]byte(value), []byte(candidate)) == 1 {
			return true, append(hashes[:index], hashes[index+1:]...)
		}
	}
	return false, hashes
}

func validTOTP(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	for offset := -1; offset <= 1; offset++ {
		if subtle.ConstantTimeCompare([]byte(totp(secret, now.Add(time.Duration(offset)*30*time.Second))), []byte(code)) == 1 {
			return true
		}
	}
	return false
}
func totp(secret string, at time.Time) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return ""
	}
	counter := uint64(at.Unix() / 30)
	buffer := make([]byte, 8)
	binary.BigEndian.PutUint64(buffer, counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(buffer)
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 15
	value := (uint32(digest[offset])&127)<<24 | (uint32(digest[offset+1])&255)<<16 | (uint32(digest[offset+2])&255)<<8 | (uint32(digest[offset+3]) & 255)
	return fmt.Sprintf("%06d", value%1000000)
}
func encryptMFASecret(value string) (string, error) {
	block, err := aes.NewCipher(mfaKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(value), nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}
func decryptMFASecret(value string) (string, error) {
	encoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(mfaKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(encoded) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid encrypted MFA secret")
	}
	plain, err := gcm.Open(nil, encoded[:gcm.NonceSize()], encoded[gcm.NonceSize():], nil)
	return string(plain), err
}
func mfaKey() []byte {
	sum := sha256.Sum256([]byte(os.Getenv("API_SESSION_SIGNING_KEY")))
	return sum[:]
}
