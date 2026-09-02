"use client";

import { CspImage as Image } from "@/components/csp-image";
import Link from "next/link";
import { FormEvent, useState } from "react";
import loginLogo from "../../asset/Logo-Login-transparent.png";

export default function ForgotPasswordPage() {
  const [message, setMessage] = useState(""); const [loading, setLoading] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) { event.preventDefault(); const form = new FormData(event.currentTarget); setLoading(true); await fetch("/api/v1/auth/forgot-password", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ tenant: form.get("tenant"), email: form.get("email") }) }); setLoading(false); setMessage("If that active account exists, a reset link is on its way."); }
  return <main className="onboarding-page"><section><Image src={loginLogo} alt="Xspace" priority /><p>ACCOUNT RECOVERY</p><h1>Reset your password</h1><span>Enter the workspace URL and email used by your account.</span><form onSubmit={submit}><label>Workspace<input name="tenant" required placeholder="your-company" /></label><label>Email<input name="email" type="email" required autoComplete="email" /></label>{message && <div className="onboarding-success" role="status">{message}</div>}<button className="login-submit" disabled={loading}>{loading ? "Sending…" : "Send reset link"}</button></form><small><Link href="/login">Back to sign in</Link></small></section></main>;
}
