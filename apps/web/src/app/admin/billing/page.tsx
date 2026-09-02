"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import styles from "./billing.module.css";

type Subscription = {
  status: string;
  trialEndsAt: string | null;
  currentPeriodEndsAt: string | null;
  cancelAtPeriodEnd: boolean;
  checkoutEnabled: boolean;
  billingProvider?: string;
  providerManaged: boolean;
  plan: {
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
  usage: {
    users: number;
    meetingsThisMonth: number;
    storageBytes: number;
    recordingsThisMonth: number;
  };
};

type Plan = Subscription["plan"];

type Invoice = {
  id: string;
  invoiceNumber: string;
  status: string;
  currency: string;
  totalAmount: number;
  hostedInvoiceUrl?: string;
  createdAt: string;
};

export default function BillingPage() {
  const [item, setItem] = useState<Subscription | null>(null);
  const [invoices, setInvoices] = useState<Invoice[]>([]);
  const [plans, setPlans] = useState<Plan[]>([]);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [saving, setSaving] = useState(false);
  const [checkoutPlan, setCheckoutPlan] = useState("");

  async function load() {
    const [subscriptionResponse, invoiceResponse] = await Promise.all([
      fetch("/api/v1/admin/subscription"),
      fetch("/api/v1/admin/billing/invoices"),
    ]);
    const subscriptionData = await subscriptionResponse
      .json()
      .catch(() => ({}));
    const invoiceData = await invoiceResponse.json().catch(() => ({}));
    if (!subscriptionResponse.ok)
      throw new Error(
        subscriptionData?.error?.message ?? "Could not load subscription",
      );
    if (!invoiceResponse.ok)
      throw new Error(invoiceData?.error?.message ?? "Could not load invoices");
    setItem(subscriptionData.subscription);
    setInvoices(invoiceData.invoices ?? []);
  }

  useEffect(() => {
    let active = true;
    void Promise.all([
      fetch("/api/v1/admin/subscription"),
      fetch("/api/v1/admin/billing/invoices"),
      fetch("/api/v1/plans"),
    ])
      .then(async ([subscriptionResponse, invoiceResponse, plansResponse]) => {
        const subscriptionData = await subscriptionResponse
          .json()
          .catch(() => ({}));
        const invoiceData = await invoiceResponse.json().catch(() => ({}));
        const plansData = await plansResponse.json().catch(() => ({}));
        if (!subscriptionResponse.ok)
          throw new Error(
            subscriptionData?.error?.message ?? "Could not load subscription",
          );
        if (!invoiceResponse.ok)
          throw new Error(
            invoiceData?.error?.message ?? "Could not load invoices",
          );
        if (!plansResponse.ok)
          throw new Error(plansData?.error?.message ?? "Could not load plans");
        if (active) {
          setItem(subscriptionData.subscription);
          setInvoices(invoiceData.invoices ?? []);
          setPlans(plansData.plans ?? []);
        }
      })
      .catch((reason) => {
        if (active)
          setError(
            reason instanceof Error ? reason.message : "Could not load billing",
          );
      });
    return () => {
      active = false;
    };
  }, []);

  async function changeCancellation(action: "cancel" | "resume") {
    setSaving(true);
    setError("");
    setNotice("");
    try {
      const response = await fetch(
        `/api/v1/admin/billing/subscription/${action}`,
        { method: "POST" },
      );
      const data = await response.json().catch(() => ({}));
      if (!response.ok)
        throw new Error(
          data?.error?.message ?? "Could not update subscription",
        );
      await load();
      setNotice(
        action === "cancel"
          ? "Cancellation scheduled for the end of the current period."
          : "Scheduled cancellation removed.",
      );
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Could not update subscription",
      );
    } finally {
      setSaving(false);
    }
  }

  async function startCheckout(planKey: string) {
    setCheckoutPlan(planKey);
    setError("");
    setNotice("");
    try {
      const response = await fetch("/api/v1/admin/billing/checkout", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ planKey }),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok)
        throw new Error(data?.error?.message ?? "Could not start checkout");
      window.location.assign(data.checkoutUrl);
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : "Could not start checkout",
      );
      setCheckoutPlan("");
    }
  }

  if (error && !item)
    return (
      <main className={styles.state}>
        <strong>Plan unavailable</strong>
        <p>{error}</p>
        <Link href="/admin">Back to admin</Link>
      </main>
    );
  if (!item)
    return (
      <main className={styles.state}>
        <p>Loading plan, usage, and invoices…</p>
      </main>
    );
  const period =
    item.status === "TRIALING" ? item.trialEndsAt : item.currentPeriodEndsAt;

  return (
    <main className={styles.page}>
      <header>
        <div>
          <p>XSPACE ADMIN · BILLING</p>
          <h1>Plan & usage</h1>
          <span>
            Current entitlements, tenant consumption, and billing history.
          </span>
        </div>
        <nav>
          <Link href="/admin">Dashboard</Link>
          <Link href="/admin/settings">Settings</Link>
          <Link href="/">← Workspace</Link>
        </nav>
      </header>
      {error && <div className={styles.error}>{error}</div>}
      {notice && <div className={styles.success}>{notice}</div>}
      <section className={styles.hero}>
        <div>
          <span className={styles.status}>{item.status}</span>
          <h2>{item.plan.name}</h2>
          <p>{item.plan.description}</p>
        </div>
        <div className={styles.price}>
          {item.plan.priceMonthlyIdr
            ? formatIDR(item.plan.priceMonthlyIdr)
            : "Custom"}
          <small>
            {item.plan.priceMonthlyIdr ? " / month" : "Contact sales"}
          </small>
        </div>
      </section>
      <section className={styles.notice}>
        <span>
          {item.status === "TRIALING" ? "Trial ends" : "Current period ends"}
        </span>
        <strong>
          {period
            ? new Intl.DateTimeFormat("en", { dateStyle: "long" }).format(
                new Date(period),
              )
            : "—"}
        </strong>
        {item.cancelAtPeriodEnd && <em>Cancellation scheduled</em>}
      </section>
      <section className={styles.grid}>
        <Usage
          label="Workspace users"
          used={item.usage.users}
          limit={item.plan.maxUsers}
        />
        <Usage
          label="Meetings this month"
          used={item.usage.meetingsThisMonth}
          limit={item.plan.maxMeetingsPerMonth}
        />
        <Usage
          label="Recordings this month"
          used={item.usage.recordingsThisMonth}
          limit={item.plan.maxRecordingsPerMonth}
        />
        <Usage
          label="Storage"
          used={item.usage.storageBytes}
          limit={item.plan.maxStorageBytes}
          bytes
        />
      </section>
      <section className={styles.plans}>
        <div className={styles.sectionTitle}>
          <div>
            <h2>Available plans</h2>
            <p>
              {item.checkoutEnabled
                ? `Secure checkout powered by ${item.billingProvider}.`
                : "Checkout will activate after Xendit credentials are configured."}
            </p>
          </div>
        </div>
        <div className={styles.planList}>
          {plans.map((plan) => (
            <article key={plan.key}>
              <div>
                <strong>{plan.name}</strong>
                <small>{plan.description}</small>
              </div>
              <b>
                {plan.priceMonthlyIdr
                  ? formatIDR(plan.priceMonthlyIdr)
                  : "Custom"}
              </b>
              {plan.key === item.plan.key &&
              item.status === "ACTIVE" &&
              !item.cancelAtPeriodEnd ? (
                <span>Current plan</span>
              ) : plan.priceMonthlyIdr === 0 ? (
                <a href="mailto:info@cankonix.com?subject=Xspace%20Enterprise">
                  Contact sales
                </a>
              ) : (
                <button
                  disabled={!item.checkoutEnabled || checkoutPlan !== ""}
                  onClick={() => void startCheckout(plan.key)}
                >
                  {checkoutPlan === plan.key
                    ? "Opening…"
                    : `Choose ${plan.name}`}
                </button>
              )}
            </article>
          ))}
        </div>
      </section>
      <section className={styles.features}>
        <h2>Included capabilities</h2>
        <div>
          {Object.entries(item.plan.features).map(([key, enabled]) => (
            <span
              className={enabled ? styles.enabled : styles.disabled}
              key={key}
            >
              {enabled ? "✓" : "—"} {featureName(key)}
            </span>
          ))}
        </div>
        <p>
          Maximum meeting duration:{" "}
          <strong>{item.plan.maxMeetingDurationMinutes} minutes</strong>
        </p>
      </section>
      <section className={styles.invoices}>
        <div className={styles.sectionTitle}>
          <div>
            <h2>Invoices</h2>
            <p>Provider-confirmed invoices will appear here automatically.</p>
          </div>
          <span>{invoices.length}</span>
        </div>
        {invoices.length === 0 ? (
          <div className={styles.empty}>No invoices yet.</div>
        ) : (
          <div className={styles.invoiceList}>
            {invoices.map((invoice) => (
              <article key={invoice.id}>
                <div>
                  <strong>{invoice.invoiceNumber || "Invoice"}</strong>
                  <small>
                    {new Intl.DateTimeFormat("en", {
                      dateStyle: "medium",
                    }).format(new Date(invoice.createdAt))}
                  </small>
                </div>
                <b>{formatMoney(invoice.totalAmount, invoice.currency)}</b>
                <span data-status={invoice.status}>{invoice.status}</span>
                {invoice.hostedInvoiceUrl && (
                  <a
                    href={invoice.hostedInvoiceUrl}
                    target="_blank"
                    rel="noreferrer"
                  >
                    View invoice
                  </a>
                )}
              </article>
            ))}
          </div>
        )}
      </section>
      <section className={styles.foot}>
        <div>
          <h2>
            {item.cancelAtPeriodEnd
              ? "Subscription ending"
              : "Subscription controls"}
          </h2>
          <p>
            {item.cancelAtPeriodEnd
              ? "Service remains available until the current paid period ends. Start a new checkout to continue afterward."
              : "Cancellation stops future billing cycles and takes effect after the current paid period."}
          </p>
        </div>
        <div className={styles.actions}>
          <a href="mailto:info@cankonix.com?subject=Xspace%20plan%20upgrade">
            Change plan
          </a>
          {!item.cancelAtPeriodEnd && (
            <button
              disabled={saving}
              onClick={() => void changeCancellation("cancel")}
            >
              {saving ? "Saving…" : "Cancel at period end"}
            </button>
          )}
          {item.cancelAtPeriodEnd && !item.providerManaged && (
            <button
              disabled={saving}
              onClick={() => void changeCancellation("resume")}
            >
              {saving ? "Saving…" : "Resume subscription"}
            </button>
          )}
        </div>
      </section>
    </main>
  );
}

function Usage({
  label,
  used,
  limit,
  bytes = false,
}: {
  label: string;
  used: number;
  limit: number;
  bytes?: boolean;
}) {
  const ratio = Math.min(100, limit ? (used / limit) * 100 : 0);
  return (
    <article>
      <div>
        <span>{label}</span>
        <strong>
          {bytes ? formatBytes(used) : used.toLocaleString()}{" "}
          <small>/ {bytes ? formatBytes(limit) : limit.toLocaleString()}</small>
        </strong>
      </div>
      <progress
        className="csp-usage-progress"
        max="100"
        value={ratio}
        aria-label={`${label} usage`}
      />
      <p>{ratio.toFixed(ratio < 1 ? 1 : 0)}% used</p>
    </article>
  );
}

function formatIDR(value: number) {
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    maximumFractionDigits: 0,
  }).format(value);
}
function formatMoney(value: number, currency: string) {
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency,
    maximumFractionDigits: 0,
  }).format(value);
}
function formatBytes(value: number) {
  if (value < 1024) return `${value} B`;
  if (value < 1024 ** 2) return `${(value / 1024).toFixed(1)} KB`;
  if (value < 1024 ** 3) return `${(value / 1024 ** 2).toFixed(1)} MB`;
  return `${(value / 1024 ** 3).toFixed(1)} GB`;
}
function featureName(key: string) {
  return (
    (
      {
        recording: "Recording",
        drive: "Drive",
        chatAttachments: "Chat attachments",
        advancedGovernance: "Advanced governance",
        sso: "SSO",
      } as Record<string, string>
    )[key] ?? key
  );
}
