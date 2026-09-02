"use client";

import { CspImage as Image } from "./csp-image";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import sidebarLogo from "../asset/Logo-transparent.png";
import thumbnailLogo from "../asset/Logo-Thumbnail-transparent.png";
import { AccountMenu } from "./account-menu";
import { NotificationCenter } from "./notification-center";
import { ThemeToggle } from "./theme-toggle";
import { GlobalSearch } from "./global-search";

type IconName = "grid" | "video" | "calendar" | "users" | "record" | "chart" | "settings" | "chat" | "arrow" | "spark" | "menu" | "close" | "more";
type NavigationItem = { icon: IconName; label: string; href: string };

const navigation: NavigationItem[] = [
  { icon: "grid", label: "Overview", href: "/" },
  { icon: "video", label: "Meetings", href: "/meetings" },
  { icon: "calendar", label: "Calendar", href: "/calendar" },
  { icon: "chat", label: "Chat", href: "/chat" },
  { icon: "grid", label: "Rooms", href: "/rooms" },
  { icon: "record", label: "Drive", href: "/drive" },
  { icon: "users", label: "People", href: "/people" },
  { icon: "record", label: "Recordings", href: "/recordings" },
  { icon: "chart", label: "Admin", href: "/admin" },
  { icon: "settings", label: "Settings", href: "/security" },
];

export function WorkspaceShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(false);
  const [meetingCount, setMeetingCount] = useState(0);
  const [role, setRole] = useState("");

  useEffect(() => {
    let active = true;
    void Promise.all([fetch("/api/v1/auth/me"), fetch("/api/v1/meetings")]).then(async ([sessionResponse, meetingsResponse]) => {
      if (sessionResponse.status === 401) {
        router.replace("/login");
        return;
      }
      const [sessionData, meetingsData] = await Promise.all([sessionResponse.json().catch(() => ({})), meetingsResponse.json().catch(() => ({}))]);
      if (active && sessionResponse.ok) setRole(sessionData.user?.role ?? "");
      if (active && meetingsResponse.ok) setMeetingCount((meetingsData.meetings ?? []).filter((item: { status: string }) => item.status !== "ENDED" && item.status !== "CANCELLED").length);
    }).catch(() => undefined);
    return () => { active = false; };
  }, [router]);

  function toggleCollapsed() {
    const next = !collapsed;
    setCollapsed(next);
    window.localStorage.setItem("xpace-sidebar-collapsed", String(next));
  }

  const canManage=role==="TENANT_ADMIN"||role==="SUPER_ADMIN";
  return <div className={`workspace-shell ${collapsed ? "sidebar-collapsed" : ""} ${mobileOpen ? "mobile-nav-open" : ""}`}>
    <aside className={`sidebar workspace-sidebar ${mobileOpen ? "sidebar-open" : ""}`}>
      <div className="sidebar-head">
        <Link className="brand" href="/" aria-label="Xspace overview"><Image src={collapsed ? thumbnailLogo : sidebarLogo} alt="Xspace" priority sizes={collapsed ? "48px" : "190px"}/></Link>
        <button className="collapse-button" aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"} onClick={toggleCollapsed}><Icon name="arrow"/></button>
        <button className="mobile-only workspace-sidebar-close" aria-label="Close navigation" onClick={() => setMobileOpen(false)}><Icon name="close"/></button>
      </div>
      <nav aria-label="Main navigation">
        <p>WORKSPACE</p>
        {navigation.slice(0, 7).map(item => <Link title={collapsed ? item.label : undefined} key={item.href} href={item.href} onClick={() => setMobileOpen(false)} className={isActive(pathname, item.href) ? "active" : ""}><Icon name={item.icon}/><span>{item.label}</span>{item.label === "Meetings" && meetingCount > 0 && <small>{meetingCount}</small>}</Link>)}
        {canManage&&<><p>MANAGE</p>{navigation.slice(7).map(item => <Link title={collapsed ? item.label : undefined} key={item.href} href={item.href} onClick={() => setMobileOpen(false)} className={isActive(pathname, item.href) ? "active" : ""}><Icon name={item.icon}/><span>{item.label}</span></Link>)}{role==="SUPER_ADMIN"&&<Link title={collapsed?"SaaS Platform":undefined} href="/platform" onClick={()=>setMobileOpen(false)} className={isActive(pathname,"/platform")?"active":""}><Icon name="spark"/><span>SaaS Platform</span></Link>}</>}
      </nav>
      {canManage&&<div className="upgrade"><Icon name="spark"/><strong>Xspace Enterprise</strong><p>Secure meeting, collaboration, governance, and content in one workspace.</p><Link href="/admin/billing">Plan & usage</Link></div>}
    </aside>
    {mobileOpen && <button aria-label="Close navigation overlay" className="scrim" onClick={() => setMobileOpen(false)}/>}
    <div className="workspace-module-main">
      <header className="workspace-account-bar"><GlobalSearch/><div className="workspace-account-actions"><NotificationCenter/><ThemeToggle/><AccountMenu/></div></header>
      <div className="workspace-mobile-bar"><button aria-label="Open navigation" onClick={() => setMobileOpen(true)}><Icon name="menu"/></button><Image src={sidebarLogo} alt="Xspace" priority sizes="130px"/><GlobalSearch compact/><NotificationCenter/><ThemeToggle compact/><AccountMenu compact/></div>
      {children}
    </div>
  </div>;
}

function isActive(pathname: string, href: string) {
  if (href === "/") return pathname === "/";
  return pathname === href || pathname.startsWith(`${href}/`);
}

function Icon({ name }: { name: IconName }) {
  const paths: Record<IconName, React.ReactNode> = {
    grid: <><rect x="3" y="3" width="7" height="7" rx="2"/><rect x="14" y="3" width="7" height="7" rx="2"/><rect x="3" y="14" width="7" height="7" rx="2"/><rect x="14" y="14" width="7" height="7" rx="2"/></>,
    video: <><rect x="3" y="6" width="13" height="12" rx="3"/><path d="m16 10 5-3v10l-5-3"/></>,
    calendar: <><rect x="3" y="5" width="18" height="16" rx="3"/><path d="M8 3v4m8-4v4M3 10h18"/></>,
    users: <><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75"/></>,
    record: <><rect x="3" y="4" width="18" height="16" rx="3"/><circle cx="12" cy="12" r="3"/></>,
    chart: <path d="M4 19V9m6 10V5m6 14v-7m6 7H2"/>,
    settings: <><circle cx="12" cy="12" r="3"/><path d="M19 15a2 2 0 0 0 .4 2l-2.4 2.4a2 2 0 0 0-2-.4 2 2 0 0 0-1 2h-4a2 2 0 0 0-1-2 2 2 0 0 0-2 .4L4.6 17A2 2 0 0 0 5 15a2 2 0 0 0-2-1v-4a2 2 0 0 0 2-1 2 2 0 0 0-.4-2L7 4.6A2 2 0 0 0 9 5a2 2 0 0 0 1-2h4a2 2 0 0 0 1 2 2 2 0 0 0 2-.4L19.4 7A2 2 0 0 0 19 9a2 2 0 0 0 2 1v4a2 2 0 0 0-2 1Z"/></>,
    chat: <><path d="M4 5.5A2.5 2.5 0 0 1 6.5 3h11A2.5 2.5 0 0 1 20 5.5v7a2.5 2.5 0 0 1-2.5 2.5H11l-4.5 4v-4A2.5 2.5 0 0 1 4 12.5Z"/><path d="M8 8h8M8 11h5"/></>,
    arrow: <path d="m9 18 6-6-6-6"/>, spark: <path d="m12 3 1.4 4.2L18 9l-4.6 1.8L12 15l-1.4-4.2L6 9l4.6-1.8L12 3Z"/>, menu: <path d="M4 7h16M4 12h16M4 17h16"/>, close: <path d="m6 6 12 12M18 6 6 18"/>, more: <><circle cx="5" cy="12" r="1" fill="currentColor"/><circle cx="12" cy="12" r="1" fill="currentColor"/><circle cx="19" cy="12" r="1" fill="currentColor"/></>,
  };
  return <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">{paths[name]}</svg>;
}
