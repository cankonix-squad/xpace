"use client";

import Image from "next/image";
import Link from "next/link";
import { FormEvent, useState } from "react";
import loginLogo from "../../asset/Logo-Login-transparent.png";

export default function SignupPage() {
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [showPasswordConfirm, setShowPasswordConfirm] = useState(false);

  async function signup(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const password = String(form.get("password") ?? "");
    const passwordConfirm = String(form.get("passwordConfirm") ?? "");
    const tenantSlug = String(form.get("tenantSlug") ?? "").trim();
    const username = String(form.get("username") ?? "").trim();
    setError(""); setMessage("");
    if (!/^[a-z0-9-]{2,48}$/.test(tenantSlug)) { setError("Workspace URL may only contain lowercase letters, numbers, and hyphens."); return; }
    if (!/^[a-z0-9._-]{2,64}$/.test(username)) { setError("Username may only contain lowercase letters, numbers, dots, underscores, and hyphens — without spaces or @."); return; }
    if (password !== passwordConfirm) { setError("Password and retype password must match."); return; }
    setLoading(true);
    const response = await fetch("/api/v1/auth/signup", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({
      tenantName: form.get("tenantName"), tenantSlug, displayName: form.get("displayName"), email: form.get("email"), username, password, passwordConfirm, termsAccepted: form.get("termsAccepted") === "on",
    }) });
    const data = await response.json().catch(() => ({})); setLoading(false);
    if (!response.ok) { setError(data?.error?.message ?? "Could not create workspace"); return; }
    setMessage(data.message ?? "Check your email to verify your account.");
    event.currentTarget.reset();
  }

  return <main className="onboarding-page"><section><Image src={loginLogo} alt="Xspace" priority /><p>START WITH XSPACE</p><h1>Create your workspace</h1><span>The first account becomes the workspace owner. Verify your email before signing in.</span>
    <form onSubmit={signup}>
      <div className="onboarding-grid"><label>Company or workspace name<input name="tenantName" required maxLength={120} autoComplete="organization" /></label><label><span className="onboarding-label">Workspace URL <FieldInfo text="Use 2–48 lowercase letters, numbers, or hyphens. Example: cankonix-team" /></span><input name="tenantSlug" required minLength={2} maxLength={48} pattern="[a-z0-9-]{2,48}" title="Use lowercase letters, numbers, and hyphens only." placeholder="your-company" aria-describedby="workspace-url-help" /><small id="workspace-url-help" className="field-hint">Lowercase only, without spaces.</small></label></div>
      <div className="onboarding-grid"><label>Your full name<input name="displayName" required maxLength={120} autoComplete="name" /></label><label><span className="onboarding-label">Username <FieldInfo text="Use 2–64 lowercase letters, numbers, dots, underscores, or hyphens. Do not enter an email address." /></span><input name="username" required minLength={2} maxLength={64} pattern="[a-z0-9._-]{2,64}" title="Use lowercase letters, numbers, dots, underscores, or hyphens — without spaces or @." autoComplete="username" placeholder="whali.isrul" aria-describedby="username-help" /><small id="username-help" className="field-hint">Example: whali.isrul — no spaces or @.</small></label></div>
      <label>Work email<input name="email" type="email" required autoComplete="email" /></label>
      <div className="onboarding-grid"><label><span className="onboarding-label">Password <FieldInfo text="Use at least 8 characters. Your password is stored as a secure Argon2id hash." /></span><PasswordInput name="password" visible={showPassword} onToggle={() => setShowPassword(value => !value)} /><small className="field-hint">Minimum 8 characters.</small></label><label>Retype password<PasswordInput name="passwordConfirm" visible={showPasswordConfirm} onToggle={() => setShowPasswordConfirm(value => !value)} /></label></div>
      <label className="terms-check"><input name="termsAccepted" type="checkbox" required /> <span>I agree to the <Link href="/legal/terms" target="_blank">Terms of Service</Link> and <Link href="/legal/privacy" target="_blank">Privacy Policy</Link>.</span></label>
      {error && <div className="login-error" role="alert">{error}</div>}{message && <div className="onboarding-success" role="status">{message}</div>}
      <button className="login-submit" disabled={loading}>{loading ? "Creating…" : "Create workspace"}</button>
    </form><small>Already have a workspace? <Link href="/login">Sign in</Link></small></section></main>;
}

function FieldInfo({ text }: { text: string }) {
  return <span className="field-info" tabIndex={0} aria-label={text}>i<span role="tooltip">{text}</span></span>;
}

function PasswordInput({ name, visible, onToggle }: { name: string; visible: boolean; onToggle: () => void }) {
  return <div className="onboarding-password"><input name={name} type={visible ? "text" : "password"} required minLength={8} autoComplete="new-password" /><button type="button" onClick={onToggle} aria-label={visible ? "Hide password" : "Show password"} aria-pressed={visible}>{visible ? <EyeOffIcon /> : <EyeIcon />}</button></div>;
}

function EyeIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M2.5 12s3.5-6 9.5-6 9.5 6 9.5 6-3.5 6-9.5 6-9.5-6-9.5-6Z"/><circle cx="12" cy="12" r="2.5"/></svg>;
}

function EyeOffIcon() {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="m3 3 18 18M10.6 6.1A9.8 9.8 0 0 1 12 6c6 0 9.5 6 9.5 6a16.8 16.8 0 0 1-2.1 2.8M6.3 6.4C3.9 8 2.5 12 2.5 12s3.5 6 9.5 6c1.4 0 2.7-.3 3.8-.8M9.9 9.8a3 3 0 0 0 4.3 4.3"/></svg>;
}
