"use client";

import { FormEvent, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { CspImage as Image } from "@/components/csp-image";
import sidebarLogo from "../asset/Logo-transparent.png";
import thumbnailLogo from "../asset/Logo-Thumbnail-transparent.png";
import { AccountMenu } from "@/components/account-menu";
import { NotificationCenter } from "@/components/notification-center";
import { ThemeToggle } from "@/components/theme-toggle";
import { GlobalSearch } from "@/components/global-search";

type IconName =
  | "grid"
  | "video"
  | "calendar"
  | "users"
  | "record"
  | "chart"
  | "settings"
  | "chat"
  | "search"
  | "bell"
  | "plus"
  | "link"
  | "clock"
  | "arrow"
  | "play"
  | "spark"
  | "menu"
  | "close"
  | "check"
  | "shield"
  | "more";
type Modal = "new" | "join" | "schedule" | null;
type SessionUser = {
  displayName: string;
  email: string;
  tenantName: string;
  role: string;
};
type WorkspaceMeeting = {
  id: string;
  title: string;
  joinCode: string;
  status: string;
  scheduledAt?: string;
  createdAt: string;
  waitingRoomEnabled: boolean;
};
type WorkspaceRecording = {
  id: string;
  status: string;
  startedAt: string;
  stoppedAt?: string;
  title: string;
  joinCode: string;
};
type DirectoryUser = { id: string; username: string; displayName: string };
type ActivityItem = {
  id: string;
  type: "meeting" | "chat" | "room" | "drive" | "calendar" | "recording";
  title: string;
  description: string;
  actorName: string;
  href: string;
  createdAt: string;
};
const navigation: [IconName, string][] = [
  ["grid", "Overview"],
  ["video", "Meetings"],
  ["calendar", "Calendar"],
  ["chat", "Chat"],
  ["grid", "Rooms"],
  ["record", "Drive"],
  ["users", "People"],
  ["record", "Recordings"],
  ["chart", "Admin"],
  ["settings", "Settings"],
];

function Icon({ name, size = 18 }: { name: IconName; size?: number }) {
  const paths: Record<IconName, React.ReactNode> = {
    grid: (
      <>
        <rect x="3" y="3" width="7" height="7" rx="2" />
        <rect x="14" y="3" width="7" height="7" rx="2" />
        <rect x="3" y="14" width="7" height="7" rx="2" />
        <rect x="14" y="14" width="7" height="7" rx="2" />
      </>
    ),
    video: (
      <>
        <rect x="3" y="6" width="13" height="12" rx="3" />
        <path d="m16 10 5-3v10l-5-3" />
      </>
    ),
    calendar: (
      <>
        <rect x="3" y="5" width="18" height="16" rx="3" />
        <path d="M8 3v4m8-4v4M3 10h18" />
      </>
    ),
    users: (
      <>
        <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
        <circle cx="9" cy="7" r="4" />
        <path d="M22 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75" />
      </>
    ),
    record: (
      <>
        <rect x="3" y="4" width="18" height="16" rx="3" />
        <circle cx="12" cy="12" r="3" />
      </>
    ),
    chart: <path d="M4 19V9m6 10V5m6 14v-7m6 7H2" />,
    settings: (
      <>
        <circle cx="12" cy="12" r="3" />
        <path d="M19 15a2 2 0 0 0 .4 2l-2.4 2.4a2 2 0 0 0-2-.4 2 2 0 0 0-1 2h-4a2 2 0 0 0-1-2 2 2 0 0 0-2 .4L4.6 17A2 2 0 0 0 5 15a2 2 0 0 0-2-1v-4a2 2 0 0 0 2-1 2 2 0 0 0-.4-2L7 4.6A2 2 0 0 0 9 5a2 2 0 0 0 1-2h4a2 2 0 0 0 1 2 2 2 0 0 0 2-.4L19.4 7A2 2 0 0 0 19 9a2 2 0 0 0 2 1v4a2 2 0 0 0-2 1Z" />
      </>
    ),
    chat: (
      <>
        <path d="M4 5.5A2.5 2.5 0 0 1 6.5 3h11A2.5 2.5 0 0 1 20 5.5v7a2.5 2.5 0 0 1-2.5 2.5H11l-4.5 4v-4h0A2.5 2.5 0 0 1 4 12.5Z" />
        <path d="M8 8h8M8 11h5" />
      </>
    ),
    search: (
      <>
        <circle cx="11" cy="11" r="7" />
        <path d="m20 20-4-4" />
      </>
    ),
    bell: (
      <>
        <path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4" />
      </>
    ),
    plus: <path d="M12 5v14M5 12h14" />,
    link: (
      <>
        <path d="M10 13a5 5 0 0 0 7 .1l2-2A5 5 0 0 0 12 4l-1 1" />
        <path d="M14 11a5 5 0 0 0-7-.1l-2 2A5 5 0 0 0 12 20l1-1" />
      </>
    ),
    clock: (
      <>
        <circle cx="12" cy="12" r="9" />
        <path d="M12 7v5l3 2" />
      </>
    ),
    arrow: <path d="m9 18 6-6-6-6" />,
    play: <path d="m9 7 8 5-8 5Z" />,
    spark: (
      <path d="m12 3 1.4 4.2L18 9l-4.6 1.8L12 15l-1.4-4.2L6 9l4.6-1.8L12 3Zm6 12 .7 2.3L21 18l-2.3.7L18 21l-.7-2.3L15 18l2.3-.7L18 15Z" />
    ),
    menu: <path d="M4 7h16M4 12h16M4 17h16" />,
    close: <path d="m6 6 12 12M18 6 6 18" />,
    check: <path d="m5 12 4 4L19 6" />,
    shield: <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10Z" />,
    more: (
      <>
        <circle cx="5" cy="12" r="1" fill="currentColor" />
        <circle cx="12" cy="12" r="1" fill="currentColor" />
        <circle cx="19" cy="12" r="1" fill="currentColor" />
      </>
    ),
  };
  return (
    <svg
      aria-hidden="true"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      {paths[name]}
    </svg>
  );
}
function Brand({ collapsed = false }: { collapsed?: boolean }) {
  return (
    <div className="brand">
      <Image
        src={collapsed ? thumbnailLogo : sidebarLogo}
        alt="Xspace"
        priority
        sizes={collapsed ? "48px" : "190px"}
      />
    </div>
  );
}

export default function Home() {
  const router = useRouter();
  const [modal, setModal] = useState<Modal>(null);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(false);
  const [toast, setToast] = useState<string | null>(null);
  const [active, setActive] = useState("Overview");
  const [sessionUser, setSessionUser] = useState<SessionUser | null>(null);
  const [workspaceMeetings, setWorkspaceMeetings] = useState<
      WorkspaceMeeting[]
    >([]),
    [workspaceRecordings, setWorkspaceRecordings] = useState<
      WorkspaceRecording[]
    >([]),
    [directory, setDirectory] = useState<DirectoryUser[]>([]),
    [overviewLoading, setOverviewLoading] = useState(true);
  const [activity, setActivity] = useState<ActivityItem[]>([]),
    [activityLoading, setActivityLoading] = useState(true),
    [activityMore, setActivityMore] = useState(false),
    [activityCursor, setActivityCursor] = useState(""),
    [activityFilter, setActivityFilter] = useState("all");
  useEffect(() => {
    let active = true;
    void fetch("/api/v1/auth/me")
      .then(async (response) => {
        if (response.status === 401) {
          router.replace("/login");
          return;
        }
        const data = await response.json();
        if (active && response.ok) setSessionUser(data.user);
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [router]);
  useEffect(() => {
    let mounted = true;
    void (async () => {
      try {
        const [meetingResponse, directoryResponse] = await Promise.all([
          fetch("/api/v1/meetings"),
          fetch("/api/v1/directory/users"),
        ]);
        const meetingData = await meetingResponse.json().catch(() => ({})),
          directoryData = await directoryResponse.json().catch(() => ({}));
        if (!meetingResponse.ok)
          throw new Error(
            meetingData?.error?.message ?? "Could not load workspace overview",
          );
        const items: WorkspaceMeeting[] = meetingData.meetings ?? [];
        if (mounted) {
          setWorkspaceMeetings(items);
          if (directoryResponse.ok) setDirectory(directoryData.users ?? []);
        }
        const recordingResults = await Promise.all(
          items.map(async (meeting) => {
            const response = await fetch(
              `/api/v1/meetings/${encodeURIComponent(meeting.joinCode)}/recordings`,
            );
            if (!response.ok) return [];
            const data = await response.json().catch(() => ({}));
            return (data.recordings ?? []).map(
              (recording: Omit<WorkspaceRecording, "title" | "joinCode">) => ({
                ...recording,
                title: meeting.title,
                joinCode: meeting.joinCode,
              }),
            );
          }),
        );
        if (mounted)
          setWorkspaceRecordings(
            recordingResults
              .flat()
              .sort((a, b) => b.startedAt.localeCompare(a.startedAt)),
          );
      } catch (reason) {
        if (mounted)
          setToast(
            reason instanceof Error
              ? reason.message
              : "Could not load workspace overview",
          );
      } finally {
        if (mounted) setOverviewLoading(false);
      }
    })();
    return () => {
      mounted = false;
    };
  }, []);
  useEffect(() => {
    let mounted = true;
    void fetch("/api/v1/activity?limit=12")
      .then(async (response) => {
        const data = await response.json().catch(() => ({}));
        if (!response.ok)
          throw new Error(
            data?.error?.message ?? "Could not load workspace activity",
          );
        if (mounted) {
          setActivity(data.activity ?? []);
          setActivityMore(Boolean(data.pagination?.hasMore));
          setActivityCursor(data.pagination?.nextBefore ?? "");
        }
      })
      .catch((reason) => {
        if (mounted)
          setToast(
            reason instanceof Error
              ? reason.message
              : "Could not load workspace activity",
          );
      })
      .finally(() => {
        if (mounted) setActivityLoading(false);
      });
    return () => {
      mounted = false;
    };
  }, []);
  useEffect(() => {
    if (!toast) return;
    const timer = window.setTimeout(() => setToast(null), 3600);
    return () => window.clearTimeout(timer);
  }, [toast]);
  useEffect(() => {
    const media = window.matchMedia("(max-width: 760px)");
    const expandForMobile = () => {
      if (media.matches) setCollapsed(false);
    };
    expandForMobile();
    media.addEventListener("change", expandForMobile);
    return () => media.removeEventListener("change", expandForMobile);
  }, []);
  const notify = (message: string) => setToast(message);
  const canManage =
    sessionUser?.role === "TENANT_ADMIN" || sessionUser?.role === "SUPER_ADMIN";
  const navTo = (label: string) => {
    const routes: Record<string, string> = {
      Meetings: "/meetings",
      Calendar: "/calendar",
      Chat: "/chat",
      Rooms: "/rooms",
      Drive: "/drive",
      People: "/people",
      Recordings: "/recordings",
      Admin: "/admin",
      Settings: "/security",
    };
    setActive(label);
    setMobileOpen(false);
    if (routes[label]) router.push(routes[label]);
  };
  async function loadMoreActivity() {
    if (!activityCursor || activityLoading) return;
    setActivityLoading(true);
    try {
      const response = await fetch(
          `/api/v1/activity?limit=12&before=${encodeURIComponent(activityCursor)}`,
        ),
        data = await response.json().catch(() => ({}));
      if (!response.ok)
        throw new Error(data?.error?.message ?? "Could not load more activity");
      setActivity((items) => [
        ...items,
        ...(data.activity ?? []).filter(
          (candidate: ActivityItem) =>
            !items.some(
              (item) =>
                item.type === candidate.type && item.id === candidate.id,
            ),
        ),
      ]);
      setActivityMore(Boolean(data.pagination?.hasMore));
      setActivityCursor(data.pagination?.nextBefore ?? "");
    } catch (reason) {
      setToast(
        reason instanceof Error
          ? reason.message
          : "Could not load more activity",
      );
    } finally {
      setActivityLoading(false);
    }
  }
  const visibleMeetings = workspaceMeetings
      .filter((item) => !["ENDED", "CANCELLED"].includes(item.status))
      .sort((a, b) => meetingDate(a).getTime() - meetingDate(b).getTime()),
    nextMeeting = visibleMeetings[0],
    recentRecordings = workspaceRecordings.slice(0, 3),
    activeMeetings = workspaceMeetings.filter(
      (item) => item.status === "ACTIVE",
    ).length,
    scheduledMeetings = workspaceMeetings.filter(
      (item) => item.status === "SCHEDULED",
    ).length,
    totalRecordedSeconds = workspaceRecordings.reduce(
      (total, item) => total + recordingSeconds(item),
      0,
    );
  const visibleActivity =
    activityFilter === "all"
      ? activity
      : activity.filter((item) => item.type === activityFilter);
  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const joining = modal === "join";
    const code = String(form.get("code") ?? "")
      .trim()
      .toUpperCase();
    const endpoint = joining
      ? `/api/v1/meetings/${encodeURIComponent(code)}`
      : "/api/v1/meetings";
    const body = joining
      ? {}
      : {
          title: form.get("title"),
          scheduledAt:
            modal === "schedule"
              ? new Date(
                  `${form.get("date")}T${form.get("time")}`,
                ).toISOString()
              : null,
          waitingRoomEnabled: form.get("waiting") === "on",
        };
    try {
      const response = await fetch(endpoint, {
        method: joining ? "GET" : "POST",
        headers: { "Content-Type": "application/json" },
        ...(joining ? {} : { body: JSON.stringify(body) }),
      });
      const result = await response.json().catch(() => ({}));
      if (!response.ok)
        throw new Error(result?.error?.message ?? "Request failed");
      setModal(null);
      if (joining || modal === "new")
        router.push(
          `/meet/${encodeURIComponent(result.meeting.joinCode)}/prejoin`,
        );
      else notify(`Meeting scheduled · ${result.meeting.joinCode}`);
    } catch (error) {
      notify(
        error instanceof Error
          ? error.message
          : "Could not connect to Xspace API",
      );
    }
  };
  return (
    <div
      className={`app-shell ${collapsed ? "sidebar-collapsed" : ""} ${mobileOpen ? "mobile-nav-open" : ""}`}
    >
      <aside className={`sidebar ${mobileOpen ? "sidebar-open" : ""}`}>
        <div className="sidebar-head">
          <Brand collapsed={collapsed} />
          <button
            className="collapse-button"
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            onClick={() => setCollapsed(!collapsed)}
          >
            <Icon name="arrow" />
          </button>
        </div>
        <nav aria-label="Main navigation">
          <p>WORKSPACE</p>
          {navigation.slice(0, 7).map(([icon, label]) => (
            <button
              title={collapsed ? label : undefined}
              key={label}
              onClick={() => navTo(label)}
              className={active === label ? "active" : ""}
            >
              <Icon name={icon} />
              <span>{label}</span>
              {label === "Meetings" && visibleMeetings.length > 0 && (
                <small>{visibleMeetings.length}</small>
              )}
            </button>
          ))}
          {canManage && (
            <>
              <p>MANAGE</p>
              {navigation.slice(7).map(([icon, label]) => (
                <button
                  title={collapsed ? label : undefined}
                  key={label}
                  onClick={() => navTo(label)}
                  className={active === label ? "active" : ""}
                >
                  <Icon name={icon} />
                  <span>{label}</span>
                </button>
              ))}
            </>
          )}
        </nav>
        {canManage && (
          <div className="upgrade">
            <Icon name="spark" />
            <strong>Xspace Enterprise</strong>
            <p>
              Secure meeting, collaboration, governance, and content in one
              workspace.
            </p>
            <button onClick={() => navTo("Admin")}>Plan & usage</button>
          </div>
        )}
      </aside>
      {mobileOpen && (
        <button
          aria-label="Close navigation overlay"
          className="scrim"
          onClick={() => setMobileOpen(false)}
        />
      )}
      <main>
        <header className="topbar">
          <button
            className="icon-button mobile-only"
            aria-label="Open navigation"
            onClick={() => setMobileOpen(true)}
          >
            <Icon name="menu" />
          </button>
          <GlobalSearch />
          <div className="top-actions">
            <NotificationCenter />
            <ThemeToggle compact />
            <button
              className="help"
              aria-label="Open Xspace help"
              onClick={() => notify("Xspace support is available 24/7.")}
            >
              ?
            </button>
            <AccountMenu compact />
          </div>
        </header>
        <div className="content">
          <section className="welcome">
            <div>
              <p className="eyebrow">
                <i />{" "}
                {new Intl.DateTimeFormat("en", {
                  weekday: "long",
                  day: "2-digit",
                  month: "long",
                })
                  .format(new Date())
                  .toUpperCase()}
              </p>
              <h1>
                {greeting()},{" "}
                {sessionUser?.displayName.split(" ")[0] ?? "there"}{" "}
                <span>👋</span>
              </h1>
              <p>
                {overviewLoading
                  ? "Loading your workspace activity…"
                  : "Your live workspace data is ready."}
              </p>
            </div>
            <div className="actions">
              <button className="secondary" onClick={() => setModal("join")}>
                <Icon name="link" /> Join with code
              </button>
              <button className="primary" onClick={() => setModal("new")}>
                <Icon name="plus" /> New meeting
              </button>
            </div>
          </section>
          <section className="hero-grid">
            <article className="next">
              <div className="orb" />
              <div className="next-top">
                <span>
                  <i /> {nextMeeting ? "NEXT MEETING" : "MEETING STATUS"}
                </span>
                <Icon name="more" />
              </div>
              {nextMeeting ? (
                <>
                  <div className="next-body">
                    <p>{formatMeetingDateTime(nextMeeting)}</p>
                    <h2>{nextMeeting.title}</h2>
                    <div>
                      <span>
                        <Icon name="clock" size={15} />{" "}
                        {meetingTiming(nextMeeting)}
                      </span>
                      <span>
                        <Icon name="shield" size={15} />{" "}
                        {nextMeeting.waitingRoomEnabled
                          ? "Waiting room on"
                          : "Direct entry"}
                      </span>
                    </div>
                  </div>
                  <div className="next-foot">
                    <div className="avatars">
                      <span>{initials(sessionUser?.displayName)}</span>
                    </div>
                    <button
                      onClick={() =>
                        router.push(`/meet/${nextMeeting.joinCode}/prejoin`)
                      }
                    >
                      <Icon name="video" /> Join meeting{" "}
                      <Icon name="arrow" size={16} />
                    </button>
                  </div>
                </>
              ) : (
                <>
                  <div className="next-body">
                    <p>WORKSPACE READY</p>
                    <h2>No upcoming meetings</h2>
                    <div>
                      <span>
                        <Icon name="clock" size={15} /> Create an instant or
                        scheduled meeting
                      </span>
                    </div>
                  </div>
                  <div className="next-foot">
                    <div />
                    <button onClick={() => setModal("new")}>
                      <Icon name="plus" /> Create meeting{" "}
                      <Icon name="arrow" size={16} />
                    </button>
                  </div>
                </>
              )}
            </article>
            <Quick
              icon="video"
              title="Start an instant meeting"
              text="Create a secure room and invite your team in seconds."
              button="Start now"
              onClick={() => setModal("new")}
            />
            <Quick
              icon="calendar"
              title="Schedule for later"
              text="Plan ahead and send calendar invitations to everyone."
              button="Schedule"
              onClick={() => setModal("schedule")}
            />
          </section>
          <section className="stats">
            {[
              [
                "calendar",
                "Upcoming",
                String(scheduledMeetings),
                `${visibleMeetings.length} available meetings`,
                "green",
              ],
              [
                "video",
                "Live now",
                String(activeMeetings),
                activeMeetings === 1
                  ? "1 active room"
                  : `${activeMeetings} active rooms`,
                "orange",
              ],
              [
                "users",
                "People",
                String(directory.length),
                "Active workspace members",
                "blue",
              ],
              [
                "record",
                "Recorded",
                String(workspaceRecordings.length),
                formatRecordedDuration(totalRecordedSeconds),
                "purple",
              ],
            ].map(([icon, label, value, hint, tone]) => (
              <article key={label}>
                <span className={tone}>
                  <Icon name={icon as IconName} />
                </span>
                <div>
                  <p>{label}</p>
                  <strong>{overviewLoading ? "—" : value}</strong>
                </div>
                <small>{hint}</small>
              </article>
            ))}
          </section>
          <section className="lower">
            <article className="panel upcoming">
              <PanelHead
                title="Upcoming meetings"
                text="Live data from your workspace"
                action="View all"
                onClick={() => navTo("Meetings")}
              />
              <div className="meeting-list">
                {visibleMeetings.length === 0 ? (
                  <p className="dashboard-empty">
                    No active or scheduled meetings.
                  </p>
                ) : (
                  visibleMeetings.slice(0, 4).map((meeting, index) => (
                    <div
                      className={`meeting meeting-accent-${index % 4}`}
                      key={meeting.id}
                    >
                      <div className="date">
                        <b>
                          {new Intl.DateTimeFormat("en", {
                            day: "2-digit",
                          }).format(meetingDate(meeting))}
                        </b>
                        <small>
                          {new Intl.DateTimeFormat("en", { month: "short" })
                            .format(meetingDate(meeting))
                            .toUpperCase()}
                        </small>
                      </div>
                      <i />
                      <div>
                        <strong>{meeting.title}</strong>
                        <p>
                          {formatMeetingDateTime(meeting)} · {meeting.joinCode}
                        </p>
                      </div>
                      <span>{meetingTiming(meeting)}</span>
                      <button
                        aria-label={`Open ${meeting.title}`}
                        onClick={() =>
                          router.push(`/meet/${meeting.joinCode}/prejoin`)
                        }
                      >
                        <Icon name="arrow" />
                      </button>
                    </div>
                  ))
                )}
              </div>
            </article>
            <article className="panel recordings">
              <PanelHead
                title="Recent recordings"
                text="Accessible recordings from your meetings"
                action="View all"
                onClick={() => navTo("Recordings")}
              />
              <div className="record-list">
                {recentRecordings.length === 0 ? (
                  <p className="dashboard-empty">
                    No accessible recordings yet.
                  </p>
                ) : (
                  recentRecordings.map((recording, index) => (
                    <button
                      key={recording.id}
                      onClick={() => navTo("Recordings")}
                    >
                      <span className={`thumb meeting-accent-${index % 4}`}>
                        <Icon name="play" />
                      </span>
                      <span>
                        <strong>{recording.title}</strong>
                        <small>
                          {formatRecordingDate(recording.startedAt)} ·{" "}
                          {recording.status} ·{" "}
                          {formatRecordedDuration(recordingSeconds(recording))}
                        </small>
                      </span>
                      <Icon name="more" />
                    </button>
                  ))
                )}
              </div>
              <div className="storage">
                <Icon name="shield" />
                <div>
                  <strong>Enterprise-grade security</strong>
                  <small>Private tenant-scoped recording access</small>
                </div>
                <i>
                  <b />
                </i>
              </div>
            </article>
          </section>
          <section className="panel activity-feed">
            <div className="activity-head">
              <div>
                <h2>Workspace activity</h2>
                <p>Updates from the collaboration spaces you can access.</p>
              </div>
              <div
                className="activity-filters"
                aria-label="Filter workspace activity"
              >
                {[
                  ["all", "All"],
                  ["meeting", "Meet"],
                  ["chat", "Chat"],
                  ["room", "Rooms"],
                  ["drive", "Drive"],
                  ["calendar", "Calendar"],
                  ["recording", "Recordings"],
                ].map(([value, label]) => (
                  <button
                    key={value}
                    className={activityFilter === value ? "active" : ""}
                    aria-pressed={activityFilter === value}
                    onClick={() => setActivityFilter(value)}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </div>
            <div className="activity-list">
              {activityLoading && activity.length === 0 ? (
                <p className="dashboard-empty">Loading workspace activity…</p>
              ) : visibleActivity.length === 0 ? (
                <p className="dashboard-empty">
                  No activity in this category yet.
                </p>
              ) : (
                visibleActivity.map((item) => (
                  <button
                    key={`${item.type}-${item.id}`}
                    onClick={() => router.push(item.href)}
                  >
                    <span className={`activity-icon ${item.type}`}>
                      <Icon name={activityIcon(item.type)} />
                    </span>
                    <span>
                      <strong>{item.title}</strong>
                      <small>
                        {item.actorName} · {item.description}
                      </small>
                    </span>
                    <time>{relativeTime(item.createdAt)}</time>
                    <Icon name="arrow" />
                  </button>
                ))
              )}
            </div>
            {activityMore && (
              <div className="activity-more">
                <button
                  disabled={activityLoading}
                  onClick={() => void loadMoreActivity()}
                >
                  {activityLoading ? "Loading…" : "Load more activity"}
                </button>
              </div>
            )}
          </section>
          <footer>
            <span>
              <i /> Workspace connected
            </span>
            <p>Securely connected · Jakarta region</p>
            <small>© 2026 Cankonix Technology</small>
          </footer>
        </div>
      </main>
      {modal && (
        <div
          className="backdrop"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget) setModal(null);
          }}
        >
          <div
            className="modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="modal-title"
          >
            <button
              className="modal-close"
              aria-label="Close"
              onClick={() => setModal(null)}
            >
              <Icon name="close" />
            </button>
            <span className="modal-icon">
              <Icon
                name={
                  modal === "schedule"
                    ? "calendar"
                    : modal === "join"
                      ? "link"
                      : "video"
                }
              />
            </span>
            <p>XSPACE SECURE MEET</p>
            <h2 id="modal-title">
              {modal === "join"
                ? "Join a meeting"
                : modal === "schedule"
                  ? "Schedule a meeting"
                  : "Start a new meeting"}
            </h2>
            <em>
              {modal === "join"
                ? "Enter the meeting code shared by your host."
                : modal === "schedule"
                  ? "Choose when your team should come together."
                  : "Your secure room will be ready in just one click."}
            </em>
            <form onSubmit={submit}>
              {modal === "join" ? (
                <label>
                  Meeting code
                  <input
                    name="code"
                    autoFocus
                    required
                    placeholder="e.g. XPC-482-190"
                    pattern="[A-Za-z0-9-]{6,}"
                  />
                </label>
              ) : (
                <>
                  <label>
                    Meeting title
                    <input
                      name="title"
                      autoFocus
                      required
                      defaultValue={
                        modal === "new"
                          ? "Instant team sync"
                          : "Team collaboration session"
                      }
                    />
                  </label>
                  {modal === "schedule" && (
                    <div className="field-row">
                      <label>
                        Date
                        <input
                          name="date"
                          required
                          type="date"
                          defaultValue={scheduleDefaults().date}
                        />
                      </label>
                      <label>
                        Time
                        <input
                          name="time"
                          required
                          type="time"
                          defaultValue={scheduleDefaults().time}
                        />
                      </label>
                    </div>
                  )}
                </>
              )}
              <label className="toggle">
                <span>
                  <strong>Waiting room</strong>
                  <small>Review guests before they enter</small>
                </span>
                <input name="waiting" type="checkbox" defaultChecked />
              </label>
              <button className="primary" type="submit">
                {modal === "join"
                  ? "Continue to preview"
                  : modal === "schedule"
                    ? "Schedule meeting"
                    : "Create secure room"}
                <Icon name="arrow" />
              </button>
            </form>
          </div>
        </div>
      )}
      {toast && (
        <div className="toast" role="status">
          <span>
            <Icon name="check" />
          </span>
          {toast}
          <button aria-label="Dismiss" onClick={() => setToast(null)}>
            <Icon name="close" />
          </button>
        </div>
      )}
    </div>
  );
}
function Quick({
  icon,
  title,
  text,
  button,
  onClick,
}: {
  icon: IconName;
  title: string;
  text: string;
  button: string;
  onClick: () => void;
}) {
  return (
    <article className="quick">
      <div>
        <span>
          <Icon name={icon} />
        </span>
        <h3>{title}</h3>
        <p>{text}</p>
      </div>
      <button onClick={onClick}>
        {button}
        <Icon name="arrow" />
      </button>
    </article>
  );
}
function PanelHead({
  title,
  text,
  action,
  onClick,
}: {
  title: string;
  text: string;
  action: string;
  onClick: () => void;
}) {
  return (
    <div className="panel-head">
      <div>
        <h2>{title}</h2>
        <p>{text}</p>
      </div>
      <button onClick={onClick}>
        {action}
        <Icon name="arrow" />
      </button>
    </div>
  );
}
function initials(value?: string) {
  const parts = (value ?? "XP").trim().split(/\s+/).filter(Boolean);
  return (
    parts
      .slice(0, 2)
      .map((part) => part[0]?.toUpperCase())
      .join("") || "XP"
  );
}
function meetingDate(meeting: WorkspaceMeeting) {
  return new Date(meeting.scheduledAt ?? meeting.createdAt);
}
function formatMeetingDateTime(meeting: WorkspaceMeeting) {
  return new Intl.DateTimeFormat("en", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(meetingDate(meeting));
}
function meetingTiming(meeting: WorkspaceMeeting) {
  if (meeting.status === "ACTIVE") return "Live now";
  if (meeting.status === "WAITING") return "Ready to join";
  const difference = meetingDate(meeting).getTime() - Date.now(),
    minutes = Math.round(difference / 60000);
  if (minutes > 0 && minutes < 60) return `Starts in ${minutes} min`;
  if (minutes >= 60 && minutes < 1440)
    return `Starts in ${Math.round(minutes / 60)}h`;
  return meeting.status.charAt(0) + meeting.status.slice(1).toLowerCase();
}
function recordingSeconds(recording: WorkspaceRecording) {
  if (!recording.stoppedAt) return 0;
  return Math.max(
    0,
    Math.round(
      (new Date(recording.stoppedAt).getTime() -
        new Date(recording.startedAt).getTime()) /
        1000,
    ),
  );
}
function formatRecordedDuration(seconds: number) {
  if (seconds <= 0) return "No completed duration";
  const hours = Math.floor(seconds / 3600),
    minutes = Math.floor((seconds % 3600) / 60);
  return hours > 0 ? `${hours}h ${minutes}m` : `${minutes}m`;
}
function formatRecordingDate(value: string) {
  return new Intl.DateTimeFormat("en", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}
function greeting() {
  const hour = new Date().getHours();
  return hour < 12
    ? "Good morning"
    : hour < 18
      ? "Good afternoon"
      : "Good evening";
}
function activityIcon(type: ActivityItem["type"]): IconName {
  return (
    {
      meeting: "video",
      chat: "chat",
      room: "grid",
      drive: "record",
      calendar: "calendar",
      recording: "play",
    } as Record<ActivityItem["type"], IconName>
  )[type];
}
function relativeTime(value: string) {
  const difference = Math.max(0, Date.now() - new Date(value).getTime()),
    minutes = Math.floor(difference / 60000);
  if (minutes < 1) return "Just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days}d ago`;
  return new Intl.DateTimeFormat("en", { dateStyle: "medium" }).format(
    new Date(value),
  );
}
function scheduleDefaults() {
  const date = new Date(Date.now() + 24 * 60 * 60 * 1000);
  date.setMinutes(Math.ceil(date.getMinutes() / 15) * 15, 0, 0);
  return {
    date: `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`,
    time: `${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`,
  };
}
