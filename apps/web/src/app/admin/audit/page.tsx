"use client";
/* eslint-disable react-hooks/set-state-in-effect -- async loader callbacks update state only after awaited fetch responses. */
import Link from "next/link";
import { FormEvent, useCallback, useEffect, useState } from "react";
import styles from "../admin.module.css";
type Event = {
  id: string;
  action: string;
  resourceType: string;
  resourceId: string | null;
  actorName: string;
  ipAddress: string | null;
  userAgent: string | null;
  metadata: Record<string, unknown>;
  createdAt: string;
};
export default function AuditLog() {
  const [events, setEvents] = useState<Event[]>([]),
    [action, setAction] = useState(""),
    [resource, setResource] = useState(""),
    [offset, setOffset] = useState(0),
    [hasMore, setHasMore] = useState(false),
    [error, setError] = useState("");
  const load = useCallback(async () => {
    const query = new URLSearchParams({ limit: "50", offset: String(offset) });
    if (action) query.set("action", action);
    if (resource) query.set("resource", resource);
    const response = await fetch(`/api/v1/admin/audit-events?${query}`),
      data = await response.json().catch(() => ({}));
    if (!response.ok)
      throw new Error(data?.error?.message ?? "Could not load audit events");
    setEvents(data.events);
    setHasMore(data.pagination.hasMore);
  }, [action, resource, offset]);
  useEffect(() => {
    void load().catch((reason) =>
      setError(
        reason instanceof Error
          ? reason.message
          : "Could not load audit events",
      ),
    );
  }, [load]);
  const filter = (event: FormEvent) => {
    event.preventDefault();
    setOffset(0);
    void load();
  };
  return (
    <main className={styles.page}>
      <header>
        <div>
          <p>XSPACE ADMIN · SECURITY</p>
          <h1>Tenant audit log</h1>
          <span>
            Immutable activity history for administration, authentication, and
            meeting moderation.
          </span>
        </div>
        <nav>
          <Link href="/admin">Dashboard</Link>
          <Link href="/admin/meetings">Meetings</Link>
          <Link href="/admin/users">Users</Link>
        </nav>
      </header>
      {error && (
        <p className="csp-admin-error">
          {error}
        </p>
      )}
      <form onSubmit={filter} className={styles.health}>
        <input
          value={action}
          onChange={(event) => setAction(event.target.value)}
          placeholder="Action prefix, e.g. user."
          aria-label="Action filter"
          className="csp-admin-input"
        />
        <select
          value={resource}
          onChange={(event) => setResource(event.target.value)}
          aria-label="Resource filter"
          className="csp-admin-input"
        >
          <option value="">All resources</option>
          {[
            "tenant",
            "session",
            "user",
            "meeting",
            "participant",
            "recording",
            "group",
          ].map((value) => (
            <option key={value}>{value}</option>
          ))}
        </select>
        <button className="csp-admin-button">Apply filters</button>
      </form>
      <section className={`${styles.panel} csp-margin-top-16`}>
        <div className={styles.panelHead}>
          <div>
            <h2>Audit events</h2>
            <p>{events.length} events on this page</p>
          </div>
        </div>
        <div className="csp-margin-top-16">
          {events.map((event) => (
            <article key={event.id} className="csp-audit-row">
              <time className="csp-muted">
                {new Intl.DateTimeFormat("en", {
                  dateStyle: "medium",
                  timeStyle: "medium",
                }).format(new Date(event.createdAt))}
              </time>
              <div>
                <strong className="csp-accent">{event.action}</strong>
                <p className="csp-audit-resource">
                  {event.resourceType}
                  {event.resourceId ? ` · ${event.resourceId}` : ""}
                </p>
                {Object.keys(event.metadata).length > 0 && (
                  <code className="csp-audit-metadata">
                    {JSON.stringify(event.metadata)}
                  </code>
                )}
              </div>
              <div>
                <strong>{event.actorName}</strong>
                <p className="csp-audit-actor">
                  {event.ipAddress ?? "No IP recorded"}
                </p>
              </div>
            </article>
          ))}
          {events.length === 0 && (
            <p className={styles.empty}>No audit events match these filters.</p>
          )}
        </div>
        <footer className="csp-admin-footer">
          <button
            disabled={offset === 0}
            onClick={() => setOffset(Math.max(0, offset - 50))}
            className="csp-admin-button"
          >
            Previous
          </button>
          <span className="csp-muted csp-small">
            Events {offset + 1}–{offset + events.length}
          </span>
          <button
            disabled={!hasMore}
            onClick={() => setOffset(offset + 50)}
            className="csp-admin-button"
          >
            Next
          </button>
        </footer>
      </section>
    </main>
  );
}
