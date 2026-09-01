"use client";

import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useState } from "react";
import cankonixLogo from "../../asset/cankonix-white-transparent.png";
import loginLogo from "../../asset/Logo-Login-transparent.png";

export default function LoginPage() {
  const router = useRouter();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [show, setShow] = useState(false);
  const [mfaRequired, setMfaRequired] = useState(false);
  const [tenant, setTenant] = useState("");
  const [identity, setIdentity] = useState("");
  const [password, setPassword] = useState("");
  const [totpCode, setTotpCode] = useState("");

  async function login(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoading(true);
    setError("");
    try {
      const response = await fetch("/api/v1/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          tenant, identity, password, totpCode,
        }),
      });
      const result = await response.json().catch(() => ({}));
      if (!response.ok) {
        if (result?.error?.code === "MFA_REQUIRED") {
          setMfaRequired(true);
          setError("Enter the code from your authenticator app or a recovery code.");
          return;
        }
        throw new Error(result?.error?.message ?? "Sign in failed");
      }
      router.push("/");
      router.refresh();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Could not connect to Xspace");
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="login-page">
      <section className="login-story">
        <div className="login-brand">
          <Image src={loginLogo} alt="Xspace — Meet, Collaborate, Communicate, Work" priority sizes="(max-width: 760px) 70vw, 520px" />
        </div>
        <div>
          <p>SECURE COLLABORATION</p>
          <h1>Work together.<br /><em>Without boundaries.</em></h1>
          <blockquote>“Xspace gives our distributed teams the confidence and clarity to move faster.”</blockquote>
          <span>Enterprise-ready · Adaptive quality · Private by design</span>
        </div>
        <footer>
          <Image src={cankonixLogo} alt="Cankonix Technology" sizes="180px" />
        </footer>
      </section>
      <section className="login-form">
        <div>
          <p className="login-kicker">WELCOME BACK</p>
          <h2>Sign in to your workspace</h2>
          <p>Continue to meetings, recordings, and your team.</p>
          <form onSubmit={login}>
            <label>Workspace<input name="tenant" value={tenant} onChange={event=>setTenant(event.target.value)} required autoComplete="organization" placeholder="Enter your workspace" /></label>
            <label>Email or username<input name="identity" value={identity} onChange={event=>setIdentity(event.target.value)} required autoComplete="username" placeholder="Enter your e-mail or username" /></label>
            <label>Password<div className="password"><input name="password" value={password} onChange={event=>setPassword(event.target.value)} required minLength={8} type={show ? "text" : "password"} autoComplete="current-password" placeholder="Enter your password" /><button type="button" onClick={() => setShow(!show)}>{show ? "Hide" : "Show"}</button></div></label>
            {mfaRequired&&<label>Authenticator or recovery code<input name="totpCode" value={totpCode} onChange={event=>setTotpCode(event.target.value)} required autoComplete="one-time-code" inputMode="numeric" autoFocus placeholder="6-digit code" /></label>}
            <div className="login-options"><label><input type="checkbox" /> Remember me</label><Link href="/forgot-password">Forgot password?</Link></div>
            {error && <div className="login-error" role="alert">{error}</div>}
            <button className="login-submit" disabled={loading}>{loading ? "Signing in..." : mfaRequired ? "Verify and sign in" : "Sign in securely"}<span>→</span></button>
          </form>
          <small className="login-help">Need a workspace? <Link href="/signup">Start free trial</Link> · <Link href="/pricing">View plans</Link> · <a href="mailto:info@cankonix.com">Contact sales</a></small>
          <small className="login-legal"><Link href="/legal/terms">Terms</Link> · <Link href="/legal/privacy">Privacy</Link> · <Link href="/legal/cookies">Cookies</Link> · <Link href="/legal/recording">Recording notice</Link></small>
        </div>
      </section>
    </main>
  );
}
