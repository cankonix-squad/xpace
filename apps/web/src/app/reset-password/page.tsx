"use client";

import Image from "next/image";
import Link from "next/link";
import { FormEvent, Suspense, useState } from "react";
import { useSearchParams } from "next/navigation";
import loginLogo from "../../asset/Logo-Login-transparent.png";

export default function ResetPasswordPage() { return <Suspense fallback={<main className="onboarding-page"><section>Loading…</section></main>}><ResetForm /></Suspense>; }
function ResetForm() {
  const token = useSearchParams().get("token") ?? ""; const [error, setError] = useState(""); const [message, setMessage] = useState(""); const [loading, setLoading] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) { event.preventDefault(); const form = new FormData(event.currentTarget); const password = String(form.get("password") ?? ""), passwordConfirm = String(form.get("passwordConfirm") ?? ""); setError(""); if (!token) { setError("Reset link is missing."); return; } if (password !== passwordConfirm) { setError("Password and retype password must match."); return; } setLoading(true); const response = await fetch("/api/v1/auth/reset-password", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ token, password, passwordConfirm }) }); const data = await response.json().catch(() => ({})); setLoading(false); if (!response.ok) { setError(data?.error?.message ?? "Could not reset password"); return; } setMessage("Password changed. All previous sessions have been signed out."); }
  return <main className="onboarding-page"><section><Image src={loginLogo} alt="Xspace" priority /><p>SECURE RESET</p><h1>Choose a new password</h1><span>The reset link is single-use and expires after 30 minutes.</span><form onSubmit={submit}><label>New password<input name="password" type="password" required minLength={8} autoComplete="new-password" /></label><label>Retype password<input name="passwordConfirm" type="password" required minLength={8} autoComplete="new-password" /></label>{error && <div className="login-error" role="alert">{error}</div>}{message && <div className="onboarding-success" role="status">{message}</div>}<button className="login-submit" disabled={loading || Boolean(message)}>{loading ? "Saving…" : "Save new password"}</button></form><small><Link href="/login">Continue to sign in</Link></small></section></main>;
}
