"use client";
/* eslint-disable react-hooks/set-state-in-effect -- async loader callbacks update state only after awaited fetch responses. */
import Link from "next/link";
import { FormEvent, useCallback, useEffect, useState } from "react";
import styles from "../admin.module.css";
type Meeting = {
  id: string;
  title: string;
  joinCode: string;
  status: string;
  hostName: string;
  participantCount: number;
  recordingCount: number;
  durationSeconds: number;
  createdAt: string;
};
type Detail = {
  meeting: Meeting;
  participantBreakdown: {
    role: string;
    status: string;
    count: number;
    averageDurationSeconds: number;
  }[];
};
export default function AdminMeetings() {
  const [meetings, setMeetings] = useState<Meeting[]>([]),
    [status, setStatus] = useState(""),
    [search, setSearch] = useState(""),
    [error, setError] = useState(""),
    [detail, setDetail] = useState<Detail | null>(null);
  const load = useCallback(async () => {
    const query = new URLSearchParams({ limit: "100" });
    if (status) query.set("status", status);
    if (search) query.set("search", search);
    const response = await fetch(`/api/v1/admin/meetings?${query}`),
      data = await response.json().catch(() => ({}));
    if (!response.ok)
      throw new Error(data?.error?.message ?? "Could not load meetings");
    setMeetings(data.meetings);
  }, [status, search]);
  useEffect(() => {
    void load().catch((reason) =>
      setError(
        reason instanceof Error ? reason.message : "Could not load meetings",
      ),
    );
  }, [load]);
  const submit = (event: FormEvent) => {
    event.preventDefault();
    void load();
  };
  const inspect = async (id: string) => {
    const response = await fetch(`/api/v1/admin/meetings/${id}`),
      data = await response.json().catch(() => ({}));
    if (!response.ok) {
      setError(data?.error?.message ?? "Could not load analytics");
      return;
    }
    setDetail(data);
  };
  return (
    <main className={styles.page}>
      <header>
        <div>
          <p>XSPACE ADMIN · MEETINGS</p>
          <h1>Meetings & analytics</h1>
          <span>Tenant-wide meeting activity and participation insights.</span>
        </div>
        <nav>
          <Link href="/admin">Dashboard</Link>
          <Link href="/admin/users">Users</Link>
          <Link href="/admin/groups">Groups</Link>
        </nav>
      </header>
      {error && (
        <p className="csp-admin-error">
          {error}
        </p>
      )}
      <form onSubmit={submit} className={styles.health}>
        <input
          aria-label="Search meetings"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder="Search title, code, or host"
          className="csp-admin-input"
        />
        <select
          aria-label="Filter status"
          value={status}
          onChange={(event) => setStatus(event.target.value)}
          className="csp-admin-select"
        >
          <option value="">All statuses</option>
          {["SCHEDULED", "WAITING", "ACTIVE", "ENDED", "CANCELLED"].map(
            (value) => (
              <option key={value}>{value}</option>
            ),
          )}
        </select>
        <button className="csp-admin-button">
          Apply
        </button>
      </form>
      <section className={`${styles.panel} csp-margin-top-16`}>
        <div className={styles.panelHead}>
          <div>
            <h2>Meeting list</h2>
            <p>{meetings.length} results</p>
          </div>
        </div>
        <div className={styles.table}>
          <div className={styles.tableHead}>
            <span>Meeting</span>
            <span>Host</span>
            <span>Participants</span>
            <span>Status</span>
            <span>Duration</span>
          </div>
          {meetings.map((meeting) => (
            <button
              className={`${styles.row} csp-meeting-row`}
              key={meeting.id}
              onClick={() => inspect(meeting.id)}
            >
              <strong>
                {meeting.title}
                <small className="csp-meeting-code">
                  {meeting.joinCode} · {meeting.recordingCount} recordings
                </small>
              </strong>
              <span>{meeting.hostName}</span>
              <span>{meeting.participantCount}</span>
              <span className={styles[meeting.status.toLowerCase()] ?? ""}>
                {meeting.status}
              </span>
              <span>{formatDuration(meeting.durationSeconds)}</span>
            </button>
          ))}
        </div>
      </section>
      {detail && (
        <div className="csp-modal-backdrop"
          onClick={(event) => {
            if (event.target === event.currentTarget) setDetail(null);
          }}
        >
          <article className={`${styles.panel} csp-meeting-detail`}>
            <div className={styles.panelHead}>
              <div>
                <h2>{detail.meeting.title}</h2>
                <p>
                  {detail.meeting.joinCode} ·{" "}
                  {formatDuration(detail.meeting.durationSeconds)}
                </p>
              </div>
              <button onClick={() => setDetail(null)}>×</button>
            </div>
            <dl className={styles.usage}>
              <div>
                <dt>Host</dt>
                <dd>{detail.meeting.hostName}</dd>
              </div>
              <div>
                <dt>Participants</dt>
                <dd>{detail.meeting.participantCount}</dd>
              </div>
              <div>
                <dt>Recordings</dt>
                <dd>{detail.meeting.recordingCount}</dd>
              </div>
            </dl>
            <h2 className="csp-subheading">Participant breakdown</h2>
            {detail.participantBreakdown.map((item, index) => (
              <p key={`${item.role}-${item.status}-${index}`} className="csp-breakdown-row">
                <span>
                  {item.role.replace("_", " ")} · {item.status}
                </span>
                <b>
                  {item.count} · avg{" "}
                  {formatDuration(item.averageDurationSeconds)}
                </b>
              </p>
            ))}
          </article>
        </div>
      )}
    </main>
  );
}
function formatDuration(seconds: number) {
  const hours = Math.floor(seconds / 3600),
    minutes = Math.floor((seconds % 3600) / 60);
  return hours ? `${hours}h ${minutes}m` : `${minutes}m`;
}
