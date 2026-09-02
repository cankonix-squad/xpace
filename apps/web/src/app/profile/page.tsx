"use client";

import { ChangeEvent, FormEvent, useEffect, useState } from "react";
import { CspImage as Image } from "@/components/csp-image";
import styles from "./profile.module.css";

type Profile = {
  userId: string;
  displayName: string;
  email: string;
  username: string;
  role: string;
  timezone: string;
  locale: string;
  bio: string;
  avatarUrl?: string;
};

export default function ProfilePage() {
  const [profile, setProfile] = useState<Profile | null>(null),
    [error, setError] = useState(""),
    [notice, setNotice] = useState(""),
    [saving, setSaving] = useState(false),
    [uploading, setUploading] = useState(false);
  useEffect(() => {
    let active = true;
    void fetch("/api/v1/profile")
      .then(async (response) => {
        const data = await response.json().catch(() => ({}));
        if (!response.ok)
          throw new Error(data?.error?.message ?? "Could not load profile");
        if (active) setProfile(data.profile);
      })
      .catch((reason) => {
        if (active)
          setError(
            reason instanceof Error ? reason.message : "Could not load profile",
          );
      });
    return () => {
      active = false;
    };
  }, []);
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!profile) return;
    setSaving(true);
    setError("");
    setNotice("");
    const response = await fetch("/api/v1/profile", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          displayName: profile.displayName,
          timezone: profile.timezone,
          locale: profile.locale,
          bio: profile.bio,
        }),
      }),
      data = await response.json().catch(() => ({}));
    setSaving(false);
    if (!response.ok) {
      setError(data?.error?.message ?? "Could not update profile");
      return;
    }
    setProfile(data.profile);
    setNotice("Profile updated.");
    window.dispatchEvent(new Event("xpace-profile-updated"));
  }
  async function upload(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    setUploading(true);
    setError("");
    setNotice("");
    const form = new FormData();
    form.set("file", file);
    const response = await fetch("/api/v1/profile/avatar", {
        method: "PUT",
        body: form,
      }),
      data = await response.json().catch(() => ({}));
    setUploading(false);
    if (!response.ok) {
      setError(data?.error?.message ?? "Could not update profile picture");
      return;
    }
    setProfile(data.profile);
    setNotice("Profile picture updated.");
    window.dispatchEvent(new Event("xpace-profile-updated"));
  }
  async function removeAvatar() {
    if (!window.confirm("Remove your current profile picture?")) return;
    const response = await fetch("/api/v1/profile/avatar", {
      method: "DELETE",
    });
    if (!response.ok) {
      const data = await response.json().catch(() => ({}));
      setError(data?.error?.message ?? "Could not remove profile picture");
      return;
    }
    setProfile((item) => (item ? { ...item, avatarUrl: undefined } : item));
    setNotice("Profile picture removed.");
    window.dispatchEvent(new Event("xpace-profile-updated"));
  }
  if (!profile)
    return (
      <main className={styles.state}>
        {error ? (
          <p role="alert">{error}</p>
        ) : (
          <>
            <i />
            <p>Loading your profile…</p>
          </>
        )}
      </main>
    );
  return (
    <main className={styles.page}>
      <header>
        <div>
          <p>XSPACE ACCOUNT</p>
          <h1>Your profile</h1>
          <span>
            Manage the identity shown across your workspace and meetings.
          </span>
        </div>
      </header>
      {(error || notice) && (
        <div
          className={error ? styles.error : styles.notice}
          role={error ? "alert" : "status"}
        >
          {error || notice}
        </div>
      )}
      <section className={styles.layout}>
        <aside className={styles.photoCard}>
          <div
            className={`${styles.avatar} csp-profile-avatar ${profile.avatarUrl ? styles.hasPhoto : ""}`}
          >
            {profile.avatarUrl ? (
              <Image
                src={profile.avatarUrl}
                alt=""
                fill
                unoptimized
                sizes="104px"
              />
            ) : (
              initials(profile.displayName)
            )}
          </div>
          <strong>{profile.displayName}</strong>
          <span>@{profile.username}</span>
          <small>{formatRole(profile.role)}</small>
          <label className={styles.upload}>
            {uploading ? "Uploading…" : "Change profile picture"}
            <input
              type="file"
              accept="image/jpeg,image/png,image/webp"
              disabled={uploading}
              onChange={(event) => void upload(event)}
            />
          </label>
          {profile.avatarUrl && (
            <button
              className={styles.remove}
              onClick={() => void removeAvatar()}
            >
              Remove picture
            </button>
          )}
          <p>JPEG, PNG, or WebP. Maximum 2 MB.</p>
        </aside>
        <form className={styles.form} onSubmit={save}>
          <h2>Profile information</h2>
          <div className={styles.identity}>
            <label>
              Email
              <input value={profile.email} disabled />
            </label>
            <label>
              Username
              <input value={`@${profile.username}`} disabled />
            </label>
          </div>
          <label>
            Display name
            <input
              value={profile.displayName}
              minLength={2}
              maxLength={80}
              required
              onChange={(event) =>
                setProfile({ ...profile, displayName: event.target.value })
              }
            />
          </label>
          <label>
            Bio
            <textarea
              value={profile.bio}
              maxLength={280}
              rows={4}
              placeholder="Tell your team a little about yourself…"
              onChange={(event) =>
                setProfile({ ...profile, bio: event.target.value })
              }
            />
            <small>{profile.bio.length}/280</small>
          </label>
          <div className={styles.identity}>
            <label>
              Timezone
              <select
                value={profile.timezone}
                onChange={(event) =>
                  setProfile({ ...profile, timezone: event.target.value })
                }
              >
                <option>Asia/Jakarta</option>
                <option>Asia/Singapore</option>
                <option>Asia/Tokyo</option>
                <option>Europe/London</option>
                <option>America/New_York</option>
              </select>
            </label>
            <label>
              Language
              <select
                value={profile.locale}
                onChange={(event) =>
                  setProfile({ ...profile, locale: event.target.value })
                }
              >
                <option value="en-ID">English (Indonesia)</option>
                <option value="id-ID">Bahasa Indonesia</option>
                <option value="en-US">English (US)</option>
              </select>
            </label>
          </div>
          <button className={styles.save} disabled={saving}>
            {saving ? "Saving…" : "Save profile"}
          </button>
        </form>
      </section>
    </main>
  );
}
function initials(value: string) {
  return (
    value
      .trim()
      .split(/\s+/)
      .slice(0, 2)
      .map((part) => part[0]?.toUpperCase())
      .join("") || "XP"
  );
}
function formatRole(value: string) {
  return value
    .toLowerCase()
    .split("_")
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");
}
