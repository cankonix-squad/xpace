package httpapi

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cankonix/xpace/api/internal/auth"
)

type mailConfig struct {
	host, username, password, from string
	port                           int
}

func loadMailConfig() (mailConfig, bool) {
	port, err := strconv.Atoi(strings.TrimSpace(os.Getenv("SMTP_PORT")))
	config := mailConfig{host: strings.TrimSpace(os.Getenv("SMTP_HOST")), port: port, username: strings.TrimSpace(os.Getenv("SMTP_USERNAME")), password: os.Getenv("SMTP_PASSWORD"), from: strings.TrimSpace(os.Getenv("SMTP_FROM"))}
	return config, err == nil && config.host != "" && config.port > 0 && config.from != ""
}

func (api *API) signup(writer http.ResponseWriter, request *http.Request) {
	if _, configured := loadMailConfig(); !configured && os.Getenv("EMAIL_DEV_RETURN_TOKEN") != "true" {
		errorJSON(writer, http.StatusServiceUnavailable, "EMAIL_DELIVERY_UNAVAILABLE", "workspace signup is temporarily unavailable")
		return
	}
	var input struct {
		TenantName, TenantSlug, DisplayName, Email, Username, Password, PasswordConfirm string
		TermsAccepted                                                                   bool
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", err.Error())
		return
	}
	input.TenantName, input.DisplayName = strings.TrimSpace(input.TenantName), strings.TrimSpace(input.DisplayName)
	input.TenantSlug, input.Email, input.Username = strings.ToLower(strings.TrimSpace(input.TenantSlug)), strings.ToLower(strings.TrimSpace(input.Email)), strings.ToLower(strings.TrimSpace(input.Username))
	if input.TenantName == "" || input.DisplayName == "" || !validUsername(input.Username) || !validSlug(input.TenantSlug) || !validSignupEmail(input.Email) || !input.TermsAccepted {
		errorJSON(writer, 400, "INVALID_INPUT", "all fields and terms acceptance are required")
		return
	}
	if input.Password != input.PasswordConfirm {
		errorJSON(writer, 400, "PASSWORD_MISMATCH", "password and retype password must match")
		return
	}
	passwordHash, err := auth.HashPassword(input.Password)
	if err != nil {
		errorJSON(writer, 400, "WEAK_PASSWORD", err.Error())
		return
	}
	token, err := randomToken(32)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not initialize signup")
		return
	}
	encryptedToken, err := encryptMFASecret(token)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not initialize signup")
		return
	}
	tx, err := api.database.BeginTx(request.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not initialize signup")
		return
	}
	defer tx.Rollback()
	var tenantID, userID string
	if err = tx.QueryRowContext(request.Context(), `INSERT INTO tenants(slug,name) VALUES($1,$2) RETURNING id`, input.TenantSlug, input.TenantName).Scan(&tenantID); err != nil {
		errorJSON(writer, 409, "WORKSPACE_UNAVAILABLE", "that workspace URL is already in use")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `INSERT INTO tenant_subscriptions(tenant_id,plan_key,status,trial_started_at,trial_ends_at) VALUES($1,'STARTER','TRIALING',NOW(),NOW()+INTERVAL '14 days')`, tenantID); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not initialize workspace trial")
		return
	}
	if err = tx.QueryRowContext(request.Context(), `INSERT INTO users(tenant_id,email,username,display_name,password_hash,status,role) VALUES($1,$2,$3,$4,$5,'INVITED','TENANT_ADMIN') RETURNING id`, tenantID, input.Email, input.Username, input.DisplayName, passwordHash).Scan(&userID); err != nil {
		errorJSON(writer, 409, "ACCOUNT_UNAVAILABLE", "could not create that owner account")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `INSERT INTO legal_acceptances(tenant_id,user_id,terms_version,privacy_version,ip_address,user_agent) VALUES($1,$2,$3,$4,$5,$6)`, tenantID, userID, currentTermsVersion(), currentPrivacyVersion(), clientIP(request), request.UserAgent()); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not record legal acceptance")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `INSERT INTO email_verification_tokens(tenant_id,user_id,token_hash,expires_at) VALUES($1,$2,$3,NOW()+INTERVAL '24 hours')`, tenantID, userID, hashToken(token)); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not create verification request")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `INSERT INTO email_outbox(tenant_id,recipient_email,template,token_encrypted) VALUES($1,$2,'VERIFY_EMAIL',$3)`, tenantID, input.Email, encryptedToken); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not queue verification email")
		return
	}
	if err = tx.Commit(); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not create workspace")
		return
	}
	response := map[string]any{"message": "Check your email to verify the workspace owner account.", "tenant": input.TenantSlug}
	if os.Getenv("EMAIL_DEV_RETURN_TOKEN") == "true" && strings.HasPrefix(os.Getenv("XPACE_PUBLIC_URL"), "http://localhost") {
		response["developmentToken"] = token
	}
	respondJSON(writer, http.StatusAccepted, response)
}

func (api *API) verifyEmail(writer http.ResponseWriter, request *http.Request) {
	var input struct{ Token string }
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", err.Error())
		return
	}
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not verify account")
		return
	}
	defer tx.Rollback()
	var tenantID, userID string
	err = tx.QueryRowContext(request.Context(), `UPDATE email_verification_tokens SET used_at=NOW() WHERE token_hash=$1 AND used_at IS NULL AND expires_at>NOW() RETURNING tenant_id,user_id`, hashToken(strings.TrimSpace(input.Token))).Scan(&tenantID, &userID)
	if err != nil {
		errorJSON(writer, 400, "INVALID_TOKEN", "verification link is invalid or expired")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `UPDATE users SET status='ACTIVE',updated_at=NOW() WHERE id=$1 AND tenant_id=$2 AND status='INVITED'`, userID, tenantID); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not activate account")
		return
	}
	if err = tx.Commit(); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not activate account")
		return
	}
	respondJSON(writer, 200, map[string]string{"message": "Email verified. You can now sign in."})
}

func (api *API) forgotPassword(writer http.ResponseWriter, request *http.Request) {
	var input struct{ Tenant, Email string }
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", err.Error())
		return
	}
	respond := func(developmentToken string) {
		response := map[string]string{"message": "If the account exists, a reset email will be sent."}
		if developmentToken != "" && os.Getenv("EMAIL_DEV_RETURN_TOKEN") == "true" && strings.HasPrefix(os.Getenv("XPACE_PUBLIC_URL"), "http://localhost") {
			response["developmentToken"] = developmentToken
		}
		respondJSON(writer, http.StatusAccepted, response)
	}
	if _, configured := loadMailConfig(); !configured && os.Getenv("EMAIL_DEV_RETURN_TOKEN") != "true" {
		respond("")
		return
	}
	var tenantID, userID, email string
	err := api.database.QueryRowContext(request.Context(), `SELECT t.id,u.id,u.email FROM tenants t JOIN users u ON u.tenant_id=t.id WHERE t.slug=$1 AND LOWER(u.email)=LOWER($2) AND u.status='ACTIVE'`, strings.ToLower(strings.TrimSpace(input.Tenant)), strings.TrimSpace(input.Email)).Scan(&tenantID, &userID, &email)
	if err != nil {
		respond("")
		return
	}
	token, err := randomToken(32)
	if err != nil {
		respond("")
		return
	}
	encrypted, err := encryptMFASecret(token)
	if err != nil {
		respond("")
		return
	}
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		respond("")
		return
	}
	defer tx.Rollback()
	_, _ = tx.ExecContext(request.Context(), `UPDATE password_reset_tokens SET used_at=NOW() WHERE tenant_id=$1 AND user_id=$2 AND used_at IS NULL`, tenantID, userID)
	if _, err = tx.ExecContext(request.Context(), `INSERT INTO password_reset_tokens(tenant_id,user_id,token_hash,expires_at) VALUES($1,$2,$3,NOW()+INTERVAL '30 minutes')`, tenantID, userID, hashToken(token)); err != nil {
		respond("")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `INSERT INTO email_outbox(tenant_id,recipient_email,template,token_encrypted) VALUES($1,$2,'RESET_PASSWORD',$3)`, tenantID, email, encrypted); err != nil {
		respond("")
		return
	}
	if tx.Commit() == nil {
		respond(token)
		return
	}
	respond("")
}

func (api *API) resetPassword(writer http.ResponseWriter, request *http.Request) {
	var input struct{ Token, Password, PasswordConfirm string }
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", err.Error())
		return
	}
	if input.Password != input.PasswordConfirm {
		errorJSON(writer, 400, "PASSWORD_MISMATCH", "password and retype password must match")
		return
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		errorJSON(writer, 400, "WEAK_PASSWORD", err.Error())
		return
	}
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not reset password")
		return
	}
	defer tx.Rollback()
	var tenantID, userID string
	err = tx.QueryRowContext(request.Context(), `UPDATE password_reset_tokens SET used_at=NOW() WHERE token_hash=$1 AND used_at IS NULL AND expires_at>NOW() RETURNING tenant_id,user_id`, hashToken(strings.TrimSpace(input.Token))).Scan(&tenantID, &userID)
	if err != nil {
		errorJSON(writer, 400, "INVALID_TOKEN", "reset link is invalid or expired")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `UPDATE users SET password_hash=$1,updated_at=NOW() WHERE id=$2 AND tenant_id=$3 AND status='ACTIVE'`, hash, userID, tenantID); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not reset password")
		return
	}
	_, _ = tx.ExecContext(request.Context(), `UPDATE sessions SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`, userID)
	var email string
	if err = tx.QueryRowContext(request.Context(), `SELECT email FROM users WHERE id=$1 AND tenant_id=$2`, userID, tenantID).Scan(&email); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not complete password reset")
		return
	}
	if err = queueTransactionalEmail(request.Context(), tx, tenantID, email, "SECURITY_NOTICE", "", "security:password-reset:"+hashToken(strings.TrimSpace(input.Token)), map[string]any{"event": "Password changed", "message": "Your Xspace password was changed and all previous sessions were signed out."}); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not queue security notice")
		return
	}
	if err = tx.Commit(); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not reset password")
		return
	}
	respondJSON(writer, 200, map[string]string{"message": "Password updated. Sign in with your new password."})
}

func validSignupEmail(value string) bool {
	parsed, err := mail.ParseAddress(value)
	return err == nil && strings.EqualFold(parsed.Address, value) && len(value) <= 254
}

func validUsername(value string) bool {
	if len(value) < 2 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '.' || character == '_' || character == '-') {
			return false
		}
	}
	return true
}

func currentTermsVersion() string {
	if value := strings.TrimSpace(os.Getenv("TERMS_VERSION")); value != "" {
		return value
	}
	return "2026-08-29"
}

func currentPrivacyVersion() string {
	if value := strings.TrimSpace(os.Getenv("PRIVACY_VERSION")); value != "" {
		return value
	}
	return "2026-08-29"
}

func StartEmailWorker(ctx context.Context, database *sql.DB, logger *slog.Logger) {
	config, configured := loadMailConfig()
	if !configured {
		logger.Warn("email worker disabled: SMTP is not configured")
		return
	}
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				deliverOneEmail(ctx, database, logger, config)
			}
		}
	}()
}

func deliverOneEmail(ctx context.Context, database *sql.DB, logger *slog.Logger, config mailConfig) {
	var id, recipient, template, encrypted string
	var payloadJSON []byte
	err := database.QueryRowContext(ctx, `UPDATE email_outbox SET status='PROCESSING',attempts=attempts+1,updated_at=NOW() WHERE id=(SELECT id FROM email_outbox WHERE status IN ('PENDING','FAILED') AND available_at<=NOW() AND attempts<5 ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING id,recipient_email,template,COALESCE(token_encrypted,''),payload`).Scan(&id, &recipient, &template, &encrypted, &payloadJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil {
		logger.Error("email outbox claim failed", "error", err)
		return
	}
	token := ""
	if encrypted != "" {
		token, err = decryptMFASecret(encrypted)
	}
	payload := map[string]any{}
	if err == nil {
		err = json.Unmarshal(payloadJSON, &payload)
	}
	payload["publicUrl"] = strings.TrimRight(os.Getenv("XPACE_PUBLIC_URL"), "/")
	if err == nil {
		err = sendTransactionalEmail(config, recipient, template, token, payload)
	}
	if err != nil {
		_, _ = database.ExecContext(ctx, `UPDATE email_outbox SET status='FAILED',last_error=$2,available_at=NOW()+INTERVAL '5 minutes',updated_at=NOW() WHERE id=$1`, id, "delivery failed")
		logger.Error("transactional email delivery failed", "outbox_id", id, "error", err)
		return
	}
	_, _ = database.ExecContext(ctx, `UPDATE email_outbox SET status='SENT',token_encrypted='',last_error=NULL,sent_at=NOW(),updated_at=NOW() WHERE id=$1`, id)
}

func sendTransactionalEmail(config mailConfig, recipient, template, token string, payload map[string]any) error {
	content, err := transactionalEmail(template, token, payload)
	if err != nil {
		return err
	}
	textBody, htmlBody := renderTransactionalEmail(content)
	boundary := "xspace-transactional-boundary"
	message := []byte("From: " + config.from + "\r\nTo: " + recipient + "\r\nSubject: " + content.Subject + "\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n--" + boundary + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + textBody + "\r\n--" + boundary + "\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n" + htmlBody + "\r\n--" + boundary + "--\r\n")
	address := net.JoinHostPort(config.host, strconv.Itoa(config.port))
	envelopeFrom := config.from
	if parsed, parseErr := mail.ParseAddress(config.from); parseErr == nil {
		envelopeFrom = parsed.Address
	}
	var smtpAuth smtp.Auth
	if config.username != "" {
		smtpAuth = smtp.PlainAuth("", config.username, config.password, config.host)
	}
	if config.port == 465 {
		connection, err := tls.Dial("tcp", address, &tls.Config{ServerName: config.host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return err
		}
		defer connection.Close()
		client, err := smtp.NewClient(connection, config.host)
		if err != nil {
			return err
		}
		defer client.Close()
		if smtpAuth != nil {
			if err = client.Auth(smtpAuth); err != nil {
				return err
			}
		}
		if err = client.Mail(envelopeFrom); err != nil {
			return err
		}
		if err = client.Rcpt(recipient); err != nil {
			return err
		}
		writer, err := client.Data()
		if err != nil {
			return err
		}
		if _, err = writer.Write(message); err != nil {
			return err
		}
		return writer.Close()
	}
	return smtp.SendMail(address, smtpAuth, envelopeFrom, []string{recipient}, message)
}
