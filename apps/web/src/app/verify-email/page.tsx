"use client";

import { CspImage as Image } from "@/components/csp-image";
import Link from "next/link";
import { Suspense, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import loginLogo from "../../asset/Logo-Login-transparent.png";

export default function VerifyEmailPage() { return <Suspense fallback={<Status text="Checking verification link…" />}><Verify /></Suspense>; }
function Verify() {
  const token = useSearchParams().get("token") ?? "";
  const [text, setText] = useState(token ? "Verifying your account…" : "Verification link is missing."); const [failed, setFailed] = useState(!token);
  useEffect(() => { if (!token) return; void fetch("/api/v1/auth/verify-email", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ token }) }).then(async response => { const data = await response.json().catch(() => ({})); if (!response.ok) { setFailed(true); setText(data?.error?.message ?? "Verification failed"); return; } setText("Email verified. Your workspace is ready."); }); }, [token]);
  return <Status text={text} failed={failed} />;
}
function Status({ text, failed = false }: { text: string; failed?: boolean }) { return <main className="onboarding-page"><section><Image src={loginLogo} alt="Xspace" priority /><p>EMAIL VERIFICATION</p><h1>{failed ? "Link not accepted" : "Almost there"}</h1><div className={failed ? "login-error" : "onboarding-success"} role="status">{text}</div><small><Link href="/login">Continue to sign in</Link></small></section></main>; }
