"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import styles from "./admin.module.css";

type Dashboard = {
  tenant: { name: string; slug: string };
  meetings: { total: number; scheduled: number; active: number; ended: number };
  usage: { users: number; participants: number; activeParticipants: number; recordings: number; recordingDurationSeconds: number; recordingStorageBytes: number };
  dailyMeetings: { date: string; count: number }[];
  recentMeetings: { id: string; title: string; status: string; hostName: string; participantCount: number; scheduledAt: string | null; createdAt: string }[];
  health: { status: string; api: string; postgres: string; databaseOpenConnections: number; checkedAt: string };
};

export default function AdminDashboard() {
  const [dashboard, setDashboard] = useState<Dashboard | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    let active = true;
    void fetch("/api/v1/admin/dashboard").then(async response => {
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data?.error?.message ?? "Could not load admin dashboard");
      if (active) setDashboard(data);
    }).catch(reason => { if (active) setError(reason instanceof Error ? reason.message : "Could not load admin dashboard"); });
    return () => { active = false; };
  }, []);
  if (error) return <main className={styles.state}><strong>Admin dashboard unavailable</strong><p>{error}</p><Link href="/">Return to workspace</Link></main>;
  if (!dashboard) return <main className={styles.state}><i/><p>Loading tenant health and usage…</p></main>;
  const peak = Math.max(1, ...dashboard.dailyMeetings.map(item => item.count));
  return <main className={styles.page}>
    <header><div><p>XPACE ADMIN</p><h1>{dashboard.tenant.name}</h1><span>Workspace operations and usage overview</span></div><nav><Link href="/admin/meetings">Meetings</Link><Link href="/admin/users">Users</Link><Link href="/admin/groups">Groups</Link><Link href="/admin/audit">Audit</Link><Link href="/admin/policies">Policies</Link><Link href="/admin/settings">Settings</Link><Link href="/">← Workspace</Link></nav></header>
    <section className={styles.health}><span><i/> All monitored services operational</span><div><b>API</b> {dashboard.health.api}</div><div><b>PostgreSQL</b> {dashboard.health.postgres}</div><div><b>DB connections</b> {dashboard.health.databaseOpenConnections}</div></section>
    <section className={styles.cards}>
      <Metric label="Total meetings" value={dashboard.meetings.total} detail={`${dashboard.meetings.active} active · ${dashboard.meetings.scheduled} scheduled`}/>
      <Metric label="Workspace users" value={dashboard.usage.users} detail={`${dashboard.usage.activeParticipants} currently joined`}/>
      <Metric label="Participants" value={dashboard.usage.participants} detail="Unique meeting participants"/>
      <Metric label="Recordings" value={dashboard.usage.recordings} detail={`${formatDuration(dashboard.usage.recordingDurationSeconds)} · ${formatBytes(dashboard.usage.recordingStorageBytes)}`}/>
    </section>
    <section className={styles.grid}>
      <article className={styles.panel}><div className={styles.panelHead}><div><h2>Meeting activity</h2><p>Meetings created over the last seven days</p></div><strong>{dashboard.meetings.ended} ended</strong></div><div className={styles.chart}>{dashboard.dailyMeetings.map(item => <div key={item.date}><span style={{height:`${Math.max(5,item.count/peak*100)}%`}} title={`${item.count} meetings`}/><small>{new Intl.DateTimeFormat("en",{weekday:"short"}).format(new Date(`${item.date}T00:00:00`))}</small></div>)}</div></article>
      <article className={styles.panel}><div className={styles.panelHead}><div><h2>Usage summary</h2><p>Tenant-scoped resource consumption</p></div></div><dl className={styles.usage}><div><dt>Recording storage</dt><dd>{formatBytes(dashboard.usage.recordingStorageBytes)}</dd></div><div><dt>Recorded duration</dt><dd>{formatDuration(dashboard.usage.recordingDurationSeconds)}</dd></div><div><dt>Active rooms</dt><dd>{dashboard.meetings.active}</dd></div><div><dt>Tenant slug</dt><dd>{dashboard.tenant.slug}</dd></div></dl></article>
    </section>
    <section className={styles.panel}><div className={styles.panelHead}><div><h2>Recent meetings</h2><p>Latest meeting activity in this tenant</p></div></div><div className={styles.table}><div className={styles.tableHead}><span>Meeting</span><span>Host</span><span>Participants</span><span>Status</span><span>Created</span></div>{dashboard.recentMeetings.length ? dashboard.recentMeetings.map(meeting => <div className={styles.row} key={meeting.id}><strong>{meeting.title}</strong><span>{meeting.hostName}</span><span>{meeting.participantCount}</span><span className={styles[meeting.status.toLowerCase()] ?? ""}>{meeting.status}</span><span>{new Intl.DateTimeFormat("en",{dateStyle:"medium"}).format(new Date(meeting.createdAt))}</span></div>) : <p className={styles.empty}>No meeting activity yet.</p>}</div></section>
  </main>;
}

function Metric({label,value,detail}:{label:string;value:number;detail:string}) { return <article><p>{label}</p><strong>{value.toLocaleString()}</strong><span>{detail}</span></article>; }
function formatDuration(seconds:number){const hours=Math.floor(seconds/3600),minutes=Math.floor(seconds%3600/60);return `${hours}h ${minutes}m`;}
function formatBytes(bytes:number){if(bytes<1024)return `${bytes} B`;if(bytes<1024**2)return `${(bytes/1024).toFixed(1)} KB`;if(bytes<1024**3)return `${(bytes/1024**2).toFixed(1)} MB`;return `${(bytes/1024**3).toFixed(1)} GB`;}
