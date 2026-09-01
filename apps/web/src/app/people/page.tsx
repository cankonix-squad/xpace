"use client";

import Image from "next/image";
import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import styles from "./people.module.css";

type Person = {
  id: string;
  username: string;
  displayName: string;
  email: string;
  role: string;
  timezone: string;
  locale: string;
  bio: string;
  avatarUrl?: string;
  createdAt: string;
};

type ViewMode = "grid" | "list";

export default function PeoplePage() {
  const router = useRouter();
  const [people, setPeople] = useState<Person[]>([]);
  const [sessionUserID, setSessionUserID] = useState("");
  const [selected, setSelected] = useState<Person | null>(null);
  const [query, setQuery] = useState("");
  const [role, setRole] = useState("ALL");
  const [view, setView] = useState<ViewMode>("grid");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [startingChat, setStartingChat] = useState(false);

  useEffect(() => {
    let mounted = true;
    void Promise.all([fetch("/api/v1/directory/users"), fetch("/api/v1/auth/me")]).then(async ([directoryResponse, sessionResponse]) => {
      const [directoryData, sessionData] = await Promise.all([directoryResponse.json().catch(() => ({})), sessionResponse.json().catch(() => ({}))]);
      if (!directoryResponse.ok) throw new Error(directoryData?.error?.message ?? "Could not load people");
      if (mounted) {
        const items:Person[]=directoryData.users ?? [];
        const params=new URLSearchParams(window.location.search),requested=params.get("userId"),search=params.get("search");
        setPeople(items);
        if(requested)setSelected(items.find(person=>person.id===requested)??null);
        if(search)setQuery(search);
        setSessionUserID(sessionData?.user?.id ?? "");
      }
    }).catch(reason => {
      if (mounted) setError(reason instanceof Error ? reason.message : "Could not load people");
    }).finally(() => {
      if (mounted) setLoading(false);
    });
    return () => { mounted = false; };
  }, []);

  const roles = useMemo(() => Array.from(new Set(people.map(person => person.role))).sort(), [people]);
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return people.filter(person => {
      const matchesQuery = !needle || `${person.displayName} ${person.username} ${person.email} ${person.role}`.toLowerCase().includes(needle);
      return matchesQuery && (role === "ALL" || person.role === role);
    });
  }, [people, query, role]);

  async function startChat(person: Person) {
    if (person.id === sessionUserID) return;
    setStartingChat(true);
    setError("");
    const response = await fetch("/api/v1/chat/conversations", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ type: "DIRECT", memberIds: [person.id] }),
    });
    const data = await response.json().catch(() => ({}));
    setStartingChat(false);
    if (!response.ok) { setError(data?.error?.message ?? "Could not start chat"); return; }
    router.push(`/chat?conversationId=${encodeURIComponent(data.conversation.id)}`);
  }

  return <main className={styles.page}>
    <header className={styles.header}>
      <div><p className={styles.eyebrow}>XSPACE DIRECTORY</p><h1>People</h1><p>Discover colleagues, view profiles, and start conversations across your workspace.</p></div>
      <div className={styles.summary}><strong>{people.length}</strong><span>Active members</span></div>
    </header>

    {error && <p className={styles.error} role="alert">{error}<button aria-label="Dismiss error" onClick={() => setError("")}>×</button></p>}

    <section className={styles.directory}>
      <div className={styles.toolbar}>
        <label className={styles.search}><span aria-hidden="true">⌕</span><input type="search" value={query} onChange={event => setQuery(event.target.value)} placeholder="Search by name, email, username, or role…" aria-label="Search workspace people" /></label>
        <select value={role} onChange={event => setRole(event.target.value)} aria-label="Filter people by role"><option value="ALL">All roles</option>{roles.map(item => <option key={item} value={item}>{formatRole(item)}</option>)}</select>
        <div className={styles.viewSwitch} aria-label="Directory view"><button className={view === "grid" ? styles.activeView : ""} aria-label="Grid view" aria-pressed={view === "grid"} onClick={() => setView("grid")}>▦</button><button className={view === "list" ? styles.activeView : ""} aria-label="List view" aria-pressed={view === "list"} onClick={() => setView("list")}>☷</button></div>
      </div>
      <div className={styles.resultMeta}><span>{filtered.length} {filtered.length === 1 ? "person" : "people"}</span>{(query || role !== "ALL") && <button onClick={() => { setQuery(""); setRole("ALL"); }}>Clear filters</button>}</div>

      {loading ? <div className={styles.empty}>Loading workspace directory…</div> : filtered.length === 0 ? <div className={styles.empty}><strong>No people found</strong><span>Try another name, email, or role.</span></div> : <div className={view === "grid" ? styles.grid : styles.rows}>
        {filtered.map(person => <button className={styles.personCard} key={person.id} onClick={() => setSelected(person)} aria-label={`View profile for ${person.displayName}`}>
          <Avatar person={person} />
          <div className={styles.identity}><strong>{person.displayName}</strong><span>@{person.username}</span><small>{person.email}</small></div>
          <div className={styles.cardFooter}><span className={styles.role}>{formatRole(person.role)}</span><span className={styles.presence}><i /> Active</span></div>
          <span className={styles.openProfile}>View profile <b>→</b></span>
        </button>)}
      </div>}
    </section>

    {selected && <div className={styles.backdrop} onMouseDown={event => { if (event.currentTarget === event.target) setSelected(null); }}>
      <aside className={styles.profilePanel} role="dialog" aria-modal="true" aria-labelledby="person-name">
        <header><span>WORKSPACE PROFILE</span><button aria-label="Close profile" onClick={() => setSelected(null)}>×</button></header>
        <section className={styles.profileHero}><Avatar person={selected} large /><div><span className={styles.presence}><i /> Available</span><h2 id="person-name">{selected.displayName}</h2><p>@{selected.username}</p><span className={styles.role}>{formatRole(selected.role)}</span></div></section>
        <div className={styles.profileActions}><button className={styles.message} disabled={selected.id === sessionUserID || startingChat} onClick={() => void startChat(selected)}>{selected.id === sessionUserID ? "This is you" : startingChat ? "Opening chat…" : "Message"}</button><a href={`mailto:${selected.email}`}>Send email</a></div>
        <section className={styles.profileSection}><h3>About</h3><p className={selected.bio ? "" : styles.muted}>{selected.bio || "This person has not added a bio yet."}</p></section>
        <section className={styles.profileSection}><h3>Contact information</h3><dl><div><dt>Email</dt><dd><a href={`mailto:${selected.email}`}>{selected.email}</a></dd></div><div><dt>Username</dt><dd>@{selected.username}</dd></div></dl></section>
        <section className={styles.profileSection}><h3>Workspace details</h3><dl><div><dt>Role</dt><dd>{formatRole(selected.role)}</dd></div><div><dt>Timezone</dt><dd>{selected.timezone}</dd></div><div><dt>Language</dt><dd>{formatLocale(selected.locale)}</dd></div><div><dt>Member since</dt><dd>{new Intl.DateTimeFormat("en", { dateStyle: "medium" }).format(new Date(selected.createdAt))}</dd></div></dl></section>
      </aside>
    </div>}
  </main>;
}

function Avatar({ person, large = false }: { person: Person; large?: boolean }) {
  return <span className={`${styles.avatar} ${large ? styles.avatarLarge : ""} ${person.avatarUrl ? styles.hasPhoto : ""}`} role="img" aria-label={`${person.displayName} profile picture`}>
    {person.avatarUrl
      ? <Image className={styles.avatarPhoto} src={person.avatarUrl} alt="" fill unoptimized sizes={large ? "104px" : "70px"} />
      : initials(person.displayName)}
    <i />
  </span>;
}

function initials(name: string) { return name.trim().split(/\s+/).slice(0, 2).map(part => part[0]?.toUpperCase()).join("") || "XP"; }
function formatRole(value: string) { return value.toLowerCase().split("_").map(part => part.charAt(0).toUpperCase() + part.slice(1)).join(" "); }
function formatLocale(value: string) { try { return new Intl.DisplayNames(["en"], { type: "language" }).of(value.split("-")[0]) ?? value; } catch { return value; } }
