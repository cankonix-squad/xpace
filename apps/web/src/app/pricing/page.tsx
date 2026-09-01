"use client";

import Image from "next/image";
import Link from "next/link";
import { useEffect, useState } from "react";
import logo from "../../asset/Logo-transparent.png";
import cankonixLogo from "../../asset/cankonix-white-transparent.png";
import styles from "./pricing.module.css";

type Plan = {
  key: string;
  name: string;
  description: string;
  priceMonthlyIdr: number;
  trialDays: number;
  maxUsers: number;
  maxMeetingsPerMonth: number;
  maxMeetingDurationMinutes: number;
  maxStorageBytes: number;
  maxRecordingsPerMonth: number;
  features: Record<string, boolean>;
};

const benefits = [
  ["Adaptive meetings", "Stable HD collaboration that adjusts automatically to each participant's network."],
  ["One secure workspace", "Meetings, team chat, rooms, files, people, and recordings stay in one tenant."],
  ["Built for control", "Waiting rooms, moderation, audit trails, retention, RBAC, MFA, SSO, and SCIM."],
  ["Ready for Indonesia", "Jakarta-hosted operations, IDR plans, local support, and enterprise onboarding."],
];

export default function PricingPage() {
  const [plans, setPlans] = useState<Plan[]>([]);
  const [error, setError] = useState("");
  useEffect(() => {
    let active = true;
    void fetch("/api/v1/plans").then(async response => {
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data?.error?.message ?? "Could not load plans");
      if (active) setPlans(data.plans ?? []);
    }).catch(reason => { if (active) setError(reason instanceof Error ? reason.message : "Could not load plans"); });
    return () => { active = false; };
  }, []);

  return <main className={styles.page}>
    <header className={styles.nav}>
      <Link href="/pricing" className={styles.brand} aria-label="Xspace pricing home"><Image src={logo} alt="Xspace" priority sizes="184px" /></Link>
      <nav aria-label="Public navigation"><a href="#platform">Platform</a><a href="#security">Security</a><a href="#plans">Pricing</a></nav>
      <div><Link className={styles.signIn} href="/login">Sign in</Link><Link className={styles.start} href="/signup">Start free trial</Link></div>
    </header>

    <section className={styles.hero}>
      <div className={styles.heroCopy}><p>SECURE COLLABORATION FOR MODERN TEAMS</p><h1>Work together.<br/><em>Without boundaries.</em></h1><span>Xspace brings meetings, conversations, files, rooms, and enterprise controls into one beautifully focused workspace.</span><div className={styles.heroActions}><Link href="/signup">Start your workspace <b>→</b></Link><a href="mailto:info@cankonix.com?subject=Xspace%20Enterprise%20Consultation">Talk to sales</a></div><small>No credit card required · {trialDays(plans)}-day trial · Cancel anytime</small></div>
      <div className={styles.heroVisual} aria-label="Xspace platform capabilities"><div className={styles.orbit}/><article><span>● LIVE</span><strong>Product roadmap sync</strong><small>8 participants · Adaptive HD</small><div><i>MI</i><i>AN</i><i>DK</i><i>+5</i></div></article><article><b>99.9%</b><small>Platform availability target</small></article><article><b>1</b><small>Secure workspace for your team</small></article></div>
    </section>

    <section className={styles.proof}><span>PRIVATE BY DESIGN</span><span>ADAPTIVE QUALITY</span><span>TENANT ISOLATION</span><span>ENTERPRISE GOVERNANCE</span></section>

    <section className={styles.platform} id="platform"><div className={styles.sectionHead}><p>XSPACE PLATFORM</p><h2>Everything your team needs.<br/>None of the clutter.</h2><span>Move from conversation to meeting to shared work without switching context.</span></div><div className={styles.benefitGrid}>{benefits.map(([title,description],index)=><article key={title}><b>0{index+1}</b><h3>{title}</h3><p>{description}</p></article>)}</div></section>

    <section className={styles.security} id="security"><div><p>ENTERPRISE SECURITY</p><h2>Your work stays yours.</h2><span>Every workspace is isolated. Access, recordings, files, identity, and lifecycle policies remain under your organization&apos;s control.</span></div><ul><li><i/>MFA, SSO OIDC, and SCIM provisioning</li><li><i/>Custom roles and granular permissions</li><li><i/>Audit trail, retention, legal hold, and export</li><li><i/>Waiting room, moderation, and secure guest access</li></ul></section>

    <section className={styles.plans} id="plans"><div className={styles.sectionHead}><p>PLANS & PRICING</p><h2>Start small. Scale with confidence.</h2><span>Plan limits below come directly from the Xspace entitlement catalog.</span></div>{error&&<p className={styles.error}>{error}</p>}<div className={styles.planGrid}>{plans.length===0&&!error?[0,1,2].map(index=><article className={styles.skeleton} key={index}/>):plans.map(plan=><PlanCard plan={plan} key={plan.key}/>)}</div></section>

    <section className={styles.finalCta}><p>READY TO WORK WITHOUT BOUNDARIES?</p><h2>Create your Xspace workspace today.</h2><span>Invite your team, run your first secure meeting, and experience one connected place for work.</span><div><Link href="/signup">Start free trial</Link><a href="mailto:info@cankonix.com?subject=Xspace%20Sales">Contact sales</a></div></section>

    <footer className={styles.footer}><Image src={cankonixLogo} alt="Cankonix Technology" sizes="160px"/><p>© 2026 Cankonix Technology. Xspace is built for secure collaboration.</p><nav><Link href="/legal">Legal</Link><Link href="/legal/privacy">Privacy</Link><Link href="/login">Sign in</Link><Link href="/signup">Create workspace</Link><a href="mailto:info@cankonix.com">info@cankonix.com</a></nav></footer>
  </main>;
}

function PlanCard({plan}:{plan:Plan}) {
  const popular = plan.key.toUpperCase() === "BUSINESS";
  const enterprise = plan.key.toUpperCase() === "ENTERPRISE" || plan.priceMonthlyIdr <= 0;
  const features = [
    `${number(plan.maxUsers)} workspace users`,
    `${number(plan.maxMeetingsPerMonth)} meetings per month`,
    `Up to ${duration(plan.maxMeetingDurationMinutes)} per meeting`,
    `${storage(plan.maxStorageBytes)} secure storage`,
    plan.features.recording ? `${number(plan.maxRecordingsPerMonth)} recordings per month` : "Meetings without recording",
    plan.features.sso ? "Enterprise SSO" : "Secure password authentication",
  ];
  return <article className={`${styles.planCard} ${popular?styles.popular:""}`}>{popular&&<span className={styles.badge}>MOST POPULAR</span>}<p>{plan.key}</p><h3>{plan.name}</h3><span>{plan.description}</span><div className={styles.price}>{enterprise?<><strong>Custom</strong><small>tailored to your organization</small></>:<><strong>{rupiah(plan.priceMonthlyIdr)}</strong><small>/ workspace / month</small></>}</div><ul>{features.map(item=><li key={item}><i>✓</i>{item}</li>)}</ul>{enterprise?<a href="mailto:info@cankonix.com?subject=Xspace%20Enterprise%20Plan">Contact sales</a>:<Link href="/signup">Start {plan.trialDays}-day trial</Link>}</article>;
}

function rupiah(value:number){return new Intl.NumberFormat("id-ID",{style:"currency",currency:"IDR",maximumFractionDigits:0}).format(value)}
function number(value:number){return new Intl.NumberFormat("id-ID").format(value)}
function duration(minutes:number){return minutes>=60?`${Math.round(minutes/60)} hours`:`${minutes} minutes`}
function storage(bytes:number){const gb=bytes/1024/1024/1024;return `${new Intl.NumberFormat("id-ID",{maximumFractionDigits:gb<10?1:0}).format(gb)} GB`}
function trialDays(plans:Plan[]){return plans.find(plan=>plan.trialDays>0)?.trialDays??14}
