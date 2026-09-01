"use client";
/* eslint-disable react-hooks/set-state-in-effect -- async loader callbacks update state only after awaited fetch responses. */
import Link from "next/link";
import { FormEvent, useCallback, useEffect, useState } from "react";
import styles from "./users.module.css";

type User = { id: string; email: string; username: string; displayName: string; role: string; status: string; createdAt: string };
type CreateMode = "ACTIVE" | "INVITED";
const roles = ["TENANT_ADMIN", "HOST", "CO_HOST", "MEMBER", "GUEST"];
const statuses = ["ACTIVE", "INVITED", "SUSPENDED", "DEACTIVATED"];

export default function UserManagement() {
  const [users, setUsers] = useState<User[]>([]);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [mode, setMode] = useState<CreateMode>("ACTIVE");
  const [showPassword, setShowPassword] = useState(false);
  const [invitationLink, setInvitationLink] = useState("");
  const [updatingUser, setUpdatingUser] = useState("");
  const load = useCallback(async () => {
    const response = await fetch("/api/v1/admin/users");
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data?.error?.message ?? "Could not load users");
    setUsers(data.users);
  }, []);
  useEffect(() => { void load().catch(reason => setError(reason instanceof Error ? reason.message : "Could not load users")); }, [load]);

  const createUser = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    const password = String(form.get("password") ?? "");
    const passwordConfirm = String(form.get("passwordConfirm") ?? "");
    setError(""); setNotice(""); setInvitationLink("");
    if (mode === "ACTIVE" && password !== passwordConfirm) { setError("Password and retype password must match."); return; }
    setBusy(true);
    const response = await fetch("/api/v1/admin/users", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ displayName: form.get("displayName"), email: form.get("email"), username: form.get("username"), role: form.get("role"), status: mode, password: mode === "ACTIVE" ? password : "", passwordConfirm: mode === "ACTIVE" ? passwordConfirm : "" }) });
    const data = await response.json().catch(() => ({}));
    setBusy(false);
    if (!response.ok) { setError(data?.error?.message ?? "Could not create user"); return; }
    formElement.reset(); setShowPassword(false);
    if (data.invitationPath) setInvitationLink(`${window.location.origin}${data.invitationPath}`);
    setNotice(mode === "ACTIVE" ? `${data.user.displayName} can now sign in.` : `${data.user.displayName} added as invited.`);
    await load();
  };

  const update = async (user: User, field: "role" | "status", value: string) => {
    setError(""); setNotice(""); setUpdatingUser(user.id);
    const response = await fetch(`/api/v1/admin/users/${encodeURIComponent(user.id)}`, { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ role: field === "role" ? value : user.role, status: field === "status" ? value : user.status }) });
    const data = await response.json().catch(() => ({}));
    setUpdatingUser("");
    if (!response.ok) { setError(data?.error?.message ?? "Could not update user"); return; }
    setUsers(items => items.map(item => item.id === user.id ? data.user : item)); setNotice(`${user.displayName} updated`);
  };

  const changeAccess = async (user: User) => {
    const activating = user.status === "DEACTIVATED";
    const targetStatus = activating ? "ACTIVE" : "DEACTIVATED";
    const action = activating ? "reactivate" : user.status === "INVITED" ? "cancel this invitation for" : "deactivate";
    if (!window.confirm(`${action.charAt(0).toUpperCase()+action.slice(1)} ${user.displayName}?${activating ? "" : " Their active sessions will be revoked."}`)) return;
    await update(user, "status", targetStatus);
  };

  const deleteUser = async (user: User) => {
    const confirmation = window.prompt(`Permanently delete ${user.displayName}? This releases their email and username, revokes access, and anonymizes their identity. Shared workspace history remains.\n\nType ${user.email} to confirm:`);
    if (confirmation === null) return;
    if (confirmation.trim().toLowerCase() !== user.email.toLowerCase()) {
      setError("Email confirmation did not match. The user was not deleted.");
      return;
    }
    setError(""); setNotice(""); setUpdatingUser(user.id);
    const response = await fetch(`/api/v1/admin/users/${encodeURIComponent(user.id)}`, { method: "DELETE" });
    const data = await response.json().catch(() => ({}));
    setUpdatingUser("");
    if (!response.ok) { setError(data?.error?.message ?? "Could not permanently delete user"); return; }
    setUsers(items => items.filter(item => item.id !== user.id));
    setNotice(`${user.displayName} permanently deleted. Their email and username can now be reused.`);
  };

  return <main className={styles.page}>
    <header><div><p>XSPACE ADMIN · PEOPLE</p><h1>User management</h1><span>Create active accounts, invite users, and control tenant access.</span></div><nav><Link href="/admin">Dashboard</Link><Link href="/">Workspace</Link></nav></header>
    {(error || notice) && <div role={error ? "alert" : "status"} aria-live="polite" className={error ? styles.error : styles.notice}>{error || notice}<button aria-label="Dismiss message" onClick={() => { setError(""); setNotice(""); }}>×</button></div>}
    <section className={styles.layout}>
      <form className={styles.invite} onSubmit={createUser}>
        <h2>{mode === "ACTIVE" ? "Create active user" : "Invite user"}</h2><p>{mode === "ACTIVE" ? "Set credentials the user can use immediately." : "Reserve an invited account for the future acceptance flow."}</p>
        <div className={styles.modeSwitch} role="group" aria-label="Account creation mode"><button type="button" className={mode === "ACTIVE" ? styles.selected : ""} aria-pressed={mode === "ACTIVE"} onClick={() => setMode("ACTIVE")}>Active user</button><button type="button" className={mode === "INVITED" ? styles.selected : ""} aria-pressed={mode === "INVITED"} onClick={() => setMode("INVITED")}>Invitation</button></div>
        <label>Display name<input name="displayName" minLength={2} maxLength={80} autoComplete="name" required /></label>
        <label>Email<input name="email" type="email" autoComplete="email" required /></label>
        <label>Username<input name="username" pattern="[A-Za-z0-9._-]+" autoComplete="username" required /></label>
        <label>Initial role<select name="role" defaultValue="MEMBER">{roles.map(role => <option key={role}>{role}</option>)}</select></label>
        {mode === "ACTIVE" && <div className={styles.passwordFields}><label>Password<div className={styles.passwordInput}><input name="password" type={showPassword ? "text" : "password"} minLength={8} autoComplete="new-password" required /><button type="button" aria-label={showPassword ? "Hide passwords" : "Show passwords"} onClick={() => setShowPassword(value => !value)}>{showPassword ? "Hide" : "Show"}</button></div><small>Minimum 8 characters.</small></label><label>Retype password<input name="passwordConfirm" type={showPassword ? "text" : "password"} minLength={8} autoComplete="new-password" required /></label></div>}
        <button disabled={busy}>{busy ? "Saving user…" : mode === "ACTIVE" ? "Create active user" : "Create invitation link"}</button>
        {invitationLink && <div className={styles.invitationResult} role="status"><strong>Invitation link (valid for 72 hours)</strong><input readOnly value={invitationLink} aria-label="Invitation link" /><button type="button" onClick={() => void navigator.clipboard.writeText(invitationLink).then(() => setNotice("Invitation link copied."))}>Copy invitation link</button><small>This link is shown once. Send it securely to the intended user.</small></div>}
      </form>
      <section className={styles.users}><div className={styles.usersHead}><div><h2>Workspace users</h2><p>{users.length} accounts in this tenant</p></div></div><div className={styles.table}><div className={styles.tableHead}><span>User</span><span>Role</span><span>Status</span><span>Created</span><span>Actions</span></div>{users.map(user => <div className={styles.row} key={user.id}><div><b>{initials(user.displayName)}</b><span><strong>{user.displayName}</strong><small>{user.email} · @{user.username}</small></span></div><select aria-label={`Role for ${user.displayName}`} value={user.role} onChange={event => void update(user, "role", event.target.value)} disabled={user.role === "SUPER_ADMIN" || updatingUser === user.id}>{user.role === "SUPER_ADMIN" && <option>SUPER_ADMIN</option>}{roles.map(role => <option key={role}>{role}</option>)}</select><select aria-label={`Status for ${user.displayName}`} value={user.status} onChange={event => void update(user, "status", event.target.value)} disabled={user.role === "SUPER_ADMIN" || updatingUser === user.id}>{statuses.map(status => <option key={status}>{status}</option>)}</select><span>{new Intl.DateTimeFormat("en", { dateStyle: "medium" }).format(new Date(user.createdAt))}</span><div className={styles.rowActions}><button className={user.status === "DEACTIVATED" ? styles.reactivate : styles.deactivate} disabled={user.role === "SUPER_ADMIN" || updatingUser === user.id} onClick={() => void changeAccess(user)}>{updatingUser === user.id ? "Saving…" : user.role === "SUPER_ADMIN" ? "Protected" : user.status === "DEACTIVATED" ? "Reactivate" : user.status === "INVITED" ? "Cancel invite" : "Deactivate"}</button>{user.role !== "SUPER_ADMIN" && <button className={styles.deleteUser} disabled={updatingUser === user.id || (user.status !== "DEACTIVATED" && user.status !== "INVITED")} title={user.status === "DEACTIVATED" || user.status === "INVITED" ? "Permanently delete this identity" : "Deactivate this account before deleting it"} onClick={() => void deleteUser(user)}>Delete permanently</button>}</div></div>)}</div><p className={styles.lifecycleNote}>Deactivate an active account first, then use <strong>Delete permanently</strong> to revoke access, anonymize its identity, and release its email and username. Shared audit, meeting, recording, chat, and file history remains intact.</p></section>
    </section>
  </main>;
}

function initials(name: string) { return name.split(/\s+/).slice(0, 2).map(part => part[0]).join("").toUpperCase(); }
