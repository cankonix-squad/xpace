"use client";

import { CspImage as Image } from "@/components/csp-image";
import { FormEvent, Suspense, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import loginLogo from "../../asset/Logo-Login-transparent.png";

export default function AcceptInvitationPage() {
  return <Suspense fallback={<main className="invite-accept-page"><section>Loading invitation…</section></main>}><AcceptInvitationForm /></Suspense>;
}

function AcceptInvitationForm() {
  const router = useRouter();
  const token = useSearchParams().get("token") ?? "";
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [show, setShow] = useState(false);

  async function accept(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const password = String(form.get("password") ?? "");
    const passwordConfirm = String(form.get("passwordConfirm") ?? "");
    setError("");
    if (!token) { setError("Invitation link is missing or invalid."); return; }
    if (password !== passwordConfirm) { setError("Password and retype password must match."); return; }
    setLoading(true);
    const response = await fetch("/api/v1/auth/invitations/accept", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ token, password, passwordConfirm }) });
    const data = await response.json().catch(() => ({}));
    setLoading(false);
    if (!response.ok) { setError(data?.error?.message ?? "Could not accept invitation"); return; }
    router.push("/"); router.refresh();
  }

  return <main className="invite-accept-page"><section><Image src={loginLogo} alt="Xspace" priority sizes="220px" /><p>WORKSPACE INVITATION</p><h1>Set your Xspace password</h1><span>Create a private password to activate your account and enter the workspace.</span><form onSubmit={accept}><label>New password<div className="password"><input name="password" required minLength={8} type={show ? "text" : "password"} autoComplete="new-password" /><button type="button" onClick={() => setShow(value => !value)}>{show ? "Hide" : "Show"}</button></div></label><label>Retype password<input name="passwordConfirm" required minLength={8} type={show ? "text" : "password"} autoComplete="new-password" /></label>{error && <div className="login-error" role="alert">{error}</div>}<button className="login-submit" disabled={loading || !token}>{loading ? "Activating…" : "Activate account"}<span>→</span></button></form><small>Invitation links expire after 72 hours and can only be used once.</small></section></main>;
}
