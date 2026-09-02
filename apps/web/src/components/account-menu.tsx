"use client";

import Link from "next/link";
import { CspImage as Image } from "./csp-image";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

type Profile = {
  displayName: string;
  email: string;
  username: string;
  role: string;
  avatarUrl?: string;
};

export function AccountMenu({ compact = false }: { compact?: boolean }) {
  const router = useRouter();
  const [profile, setProfile] = useState<Profile | null>(null),
    [open, setOpen] = useState(false);
  useEffect(() => {
    let active = true;
    const load = () => {
      void fetch("/api/v1/profile")
        .then(async (response) => {
          const data = await response.json().catch(() => ({}));
          if (active && response.ok) setProfile(data.profile);
        })
        .catch(() => undefined);
    };
    load();
    window.addEventListener("xpace-profile-updated", load);
    return () => {
      active = false;
      window.removeEventListener("xpace-profile-updated", load);
    };
  }, []);
  async function logout() {
    await fetch("/api/v1/auth/logout", { method: "POST" }).catch(
      () => undefined,
    );
    router.replace("/login");
    router.refresh();
  }
  return (
    <div className={`account-menu ${compact ? "compact" : ""}`}>
      <button
        className="account-trigger"
        aria-label="Open user profile menu"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        <Avatar profile={profile} />
        {!compact && (
          <span className="account-identity">
            <strong>{profile?.displayName ?? "Loading profile…"}</strong>
            <small>{formatRole(profile?.role)}</small>
          </span>
        )}
        <span className="account-chevron">⌄</span>
      </button>
      {open && (
        <>
          <button
            className="account-scrim"
            aria-label="Close profile menu"
            onClick={() => setOpen(false)}
          />
          <section className="account-dropdown">
            <header>
              <Avatar profile={profile} large />
              <div>
                <strong>{profile?.displayName ?? "Xspace user"}</strong>
                <small>{profile?.email ?? "Secure account"}</small>
                <em>@{profile?.username ?? "user"}</em>
              </div>
            </header>
            <nav aria-label="Account navigation">
              {profile?.role === "SUPER_ADMIN" && (
                <Link href="/platform" onClick={() => setOpen(false)}>
                  SaaS platform administration <span>›</span>
                </Link>
              )}
              <Link href="/profile" onClick={() => setOpen(false)}>
                View and edit profile <span>›</span>
              </Link>
              <Link href="/security" onClick={() => setOpen(false)}>
                Security and devices <span>›</span>
              </Link>
              <button onClick={() => void logout()}>
                Sign out <span>›</span>
              </button>
            </nav>
          </section>
        </>
      )}
    </div>
  );
}

function Avatar({
  profile,
  large = false,
}: {
  profile: Profile | null;
  large?: boolean;
}) {
  return (
    <span
      className={`account-avatar ${large ? "large" : ""} ${profile?.avatarUrl ? "has-photo" : ""}`}
    >
      {profile?.avatarUrl ? (
        <Image
          src={profile.avatarUrl}
          alt=""
          fill
          unoptimized
          sizes={large ? "48px" : "34px"}
        />
      ) : (
        initials(profile?.displayName)
      )}
      <i />
    </span>
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
function formatRole(value?: string) {
  return (value ?? "Workspace member")
    .toLowerCase()
    .split("_")
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");
}
