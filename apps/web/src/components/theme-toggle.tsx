"use client";

import { useSyncExternalStore } from "react";

type Theme = "dark-green" | "light";

function subscribe(callback: () => void) {
  window.addEventListener("xpace-theme-change", callback);
  return () => window.removeEventListener("xpace-theme-change", callback);
}

function currentTheme(): Theme {
  return document.documentElement.dataset.theme === "light" ? "light" : "dark-green";
}

export function ThemeToggle({ compact = false }: { compact?: boolean }) {
  const theme = useSyncExternalStore(subscribe, currentTheme, () => "dark-green");
  const light = theme === "light";

  function toggleTheme() {
    const next: Theme = light ? "dark-green" : "light";
    document.documentElement.dataset.theme = next;
    window.localStorage.setItem("xpace-theme", next);
    window.dispatchEvent(new Event("xpace-theme-change"));
  }

  return <button className={`theme-toggle ${compact ? "compact" : ""}`} type="button" onClick={toggleTheme} aria-label={light ? "Use Dark Green theme" : "Use Light theme"} title={light ? "Dark Green theme" : "Light theme"}>
    <span aria-hidden="true">{light ? <MoonIcon/> : <SunIcon/>}</span>
    {!compact && <small>{light ? "Light" : "Dark Green"}</small>}
  </button>;
}

function SunIcon() {
  return <svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3.5"/><path d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>;
}

function MoonIcon() {
  return <svg viewBox="0 0 24 24"><path d="M20.5 15.2A8.7 8.7 0 0 1 8.8 3.5 9 9 0 1 0 20.5 15.2Z"/></svg>;
}
