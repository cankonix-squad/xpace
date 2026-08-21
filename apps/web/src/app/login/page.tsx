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

  async function login(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoading(true);
    setError("");
    const data = new FormData(event.currentTarget);
    try {
      const response = await fetch("/api/v1/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          tenant: data.get("tenant"), identity: data.get("identity"), password: data.get("password"),
        }),
      });
      const result = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(result?.error?.message ?? "Sign in failed");
      router.push("/");
      router.refresh();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Could not connect to Xpace");
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="login-page">
      <section className="login-story">
        <div className="login-brand">
          <Image src={loginLogo} alt="Xpace — Meet, Collaborate, Communicate, Work" priority sizes="(max-width: 760px) 70vw, 520px" />
        </div>
        <div>
          <p>SECURE COLLABORATION</p>
          <h1>Work together.<br /><em>Without boundaries.</em></h1>
          <blockquote>“Xpace gives our distributed teams the confidence and clarity to move faster.”</blockquote>
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
            <label>Workspace<input name="tenant" required autoComplete="organization" placeholder="cankonix" /></label>
            <label>Email or username<input name="identity" required autoComplete="username" placeholder="admin@company.com" /></label>
            <label>Password<div className="password"><input name="password" required minLength={8} type={show ? "text" : "password"} autoComplete="current-password" placeholder="Enter your password" /><button type="button" onClick={() => setShow(!show)}>{show ? "Hide" : "Show"}</button></div></label>
            <div className="login-options"><label><input type="checkbox" /> Remember me</label><button type="button">Forgot password?</button></div>
            {error && <div className="login-error" role="alert">{error}</div>}
            <button className="login-submit" disabled={loading}>{loading ? "Signing in..." : "Sign in securely"}<span>→</span></button>
          </form>
          <small className="login-help">Need a workspace? Contact <a href="mailto:sales@cankonix.id">Xpace sales</a></small>
          <Link className="back-dashboard" href="/">← View product demo</Link>
        </div>
      </section>
    </main>
  );
}
