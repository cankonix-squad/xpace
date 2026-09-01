package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strings"
)

type emailExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type transactionalEmailContent struct {
	Subject, Heading, Message, Action, Link string
}

func queueTransactionalEmail(ctx context.Context, executor emailExecer, tenantID, recipient, template, token, dedupeKey string, payload map[string]any) error {
	encrypted := ""
	var err error
	if token != "" {
		encrypted, err = encryptMFASecret(token)
		if err != nil {
			return err
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO email_outbox(tenant_id,recipient_email,template,token_encrypted,payload,dedupe_key) VALUES($1,$2,$3,$4,$5,NULLIF($6,'')) ON CONFLICT(dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING`, tenantID, strings.ToLower(strings.TrimSpace(recipient)), template, encrypted, encoded, dedupeKey)
	return err
}

func queueWorkspaceAdminNotice(ctx context.Context, executor emailExecer, tenantID, template, dedupePrefix string, payload map[string]any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO email_outbox(tenant_id,recipient_email,template,token_encrypted,payload,dedupe_key) SELECT $1,u.email,$2,'',$3,$4||':'||u.id::text FROM users u WHERE u.tenant_id=$1 AND u.status='ACTIVE' AND u.deleted_at IS NULL AND u.role IN ('TENANT_ADMIN','SUPER_ADMIN') ON CONFLICT(dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING`, tenantID, template, encoded, dedupePrefix)
	return err
}

func transactionalEmail(template, token string, payload map[string]any) (transactionalEmailContent, error) {
	base := strings.TrimRight(getString(payload, "publicUrl"), "/")
	if base == "" {
		return transactionalEmailContent{}, fmt.Errorf("public URL is not configured")
	}
	switch template {
	case "VERIFY_EMAIL":
		return transactionalEmailContent{Subject: "Verify your Xspace workspace", Heading: "Verify your email", Message: "Confirm your email address to activate your Xspace workspace.", Action: "Verify email", Link: tokenLink(base, "/verify-email", token)}, nil
	case "RESET_PASSWORD":
		return transactionalEmailContent{Subject: "Reset your Xspace password", Heading: "Reset your password", Message: "A password reset was requested for your Xspace account. This secure link expires shortly.", Action: "Reset password", Link: tokenLink(base, "/reset-password", token)}, nil
	case "INVITATION":
		workspace := getString(payload, "workspace")
		return transactionalEmailContent{Subject: "You're invited to " + workspace + " on Xspace", Heading: "Join " + workspace, Message: getString(payload, "inviter") + " invited you to collaborate in the " + workspace + " workspace.", Action: "Accept invitation", Link: tokenLink(base, "/accept-invite", token)}, nil
	case "BILLING_NOTICE":
		return transactionalEmailContent{Subject: "Xspace billing update · " + getString(payload, "event"), Heading: "Billing update", Message: getString(payload, "message"), Action: "View plan and billing", Link: base + "/admin/billing"}, nil
	case "SECURITY_NOTICE":
		return transactionalEmailContent{Subject: "Xspace security notice · " + getString(payload, "event"), Heading: "Security notice", Message: getString(payload, "message"), Action: "Review security settings", Link: base + "/security"}, nil
	case "INCIDENT_NOTICE":
		return transactionalEmailContent{Subject: "Xspace incident escalation · " + getString(payload, "severity") + " · " + getString(payload, "title"), Heading: "Incident requires acknowledgement", Message: getString(payload, "message"), Action: "Open incident response", Link: base + "/admin/incidents"}, nil
	default:
		return transactionalEmailContent{}, fmt.Errorf("unsupported email template %q", template)
	}
}

func tokenLink(base, path, token string) string {
	return base + path + "?token=" + url.QueryEscape(token)
}

func getString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func renderTransactionalEmail(content transactionalEmailContent) (string, string) {
	textBody := content.Heading + "\r\n\r\n" + content.Message + "\r\n\r\n" + content.Action + ": " + content.Link + "\r\n\r\nIf you did not expect this email, contact your workspace administrator.\r\n"
	htmlBody := `<!doctype html><html><body style="margin:0;background:#0d120f;color:#f5f7f4;font-family:Helvetica,Arial,sans-serif"><table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td style="padding:32px 16px"><table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:620px;margin:auto;background:#151c17;border:1px solid #31402f;border-radius:16px"><tr><td style="padding:32px"><p style="margin:0 0 28px;color:#a9e653;font-size:18px;font-weight:700">Xspace</p><h1 style="margin:0 0 16px;font-size:28px">` + html.EscapeString(content.Heading) + `</h1><p style="margin:0 0 28px;color:#b8c0b9;font-size:16px;line-height:1.6">` + html.EscapeString(content.Message) + `</p><a href="` + html.EscapeString(content.Link) + `" style="display:inline-block;padding:13px 20px;border-radius:9px;background:#a9e653;color:#10150f;text-decoration:none;font-weight:700">` + html.EscapeString(content.Action) + `</a><p style="margin:28px 0 0;color:#788079;font-size:12px;line-height:1.5">If you did not expect this email, contact your workspace administrator.</p></td></tr></table></td></tr></table></body></html>`
	return textBody, htmlBody
}
