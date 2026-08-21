"use client";

import Link from "next/link";
import { FormEvent, useEffect, useState } from "react";
import styles from "../users/users.module.css";

type Configuration = {
  workspaceName: string;
  defaultTimezone: string;
  defaultLocale: string;
  supportEmail: string;
  maxMeetingDurationMinutes: number;
  recordingRetentionDays: number;
};

const defaults: Configuration = {workspaceName:"",defaultTimezone:"Asia/Jakarta",defaultLocale:"id-ID",supportEmail:"",maxMeetingDurationMinutes:120,recordingRetentionDays:30};

export default function SystemSettings() {
  const [configuration,setConfiguration]=useState<Configuration>(defaults);
  const [loading,setLoading]=useState(true);
  const [saving,setSaving]=useState(false);
  const [message,setMessage]=useState("");
  useEffect(()=>{let active=true;void fetch("/api/v1/admin/system-configuration").then(async response=>{const data=await response.json().catch(()=>({}));if(!response.ok)throw new Error(data?.error?.message??"Could not load system configuration");if(active)setConfiguration(data.configuration)}).catch(reason=>{if(active)setMessage(reason instanceof Error?reason.message:"Could not load system configuration")}).finally(()=>{if(active)setLoading(false)});return()=>{active=false}},[]);
  async function save(event:FormEvent<HTMLFormElement>){event.preventDefault();setSaving(true);const response=await fetch("/api/v1/admin/system-configuration",{method:"PUT",headers:{"Content-Type":"application/json"},body:JSON.stringify(configuration)}),data=await response.json().catch(()=>({}));setSaving(false);if(response.ok)setConfiguration(data.configuration);setMessage(response.ok?"System configuration saved":data?.error?.message??"Could not save system configuration")}
  return <main className={styles.page}><header><div><p>XPACE ADMIN · SETTINGS</p><h1>System configuration</h1><span>Workspace identity, regional defaults, and operational limits.</span></div><nav><Link href="/admin">Dashboard</Link><Link href="/admin/policies">Meeting policy</Link><Link href="/admin/audit">Audit log</Link></nav></header>{message&&<div className={styles.notice}>{message}<button type="button" onClick={()=>setMessage("")} aria-label="Dismiss message">×</button></div>}<section className={styles.users} style={{maxWidth:780}}><div className={styles.usersHead}><div><h2>Workspace defaults</h2><p>These values are isolated to this tenant and visible only to workspace administrators.</p></div></div>{loading?<p>Loading configuration…</p>:<form className={styles.invite} style={{border:0,padding:0}} onSubmit={save}><label>Workspace name<input required minLength={2} maxLength={120} value={configuration.workspaceName} onChange={event=>setConfiguration({...configuration,workspaceName:event.target.value})}/></label><label>Default timezone<input required value={configuration.defaultTimezone} onChange={event=>setConfiguration({...configuration,defaultTimezone:event.target.value})} placeholder="Asia/Jakarta"/></label><label>Default locale<input required value={configuration.defaultLocale} onChange={event=>setConfiguration({...configuration,defaultLocale:event.target.value})} placeholder="id-ID"/></label><label>Support email<input type="email" value={configuration.supportEmail} onChange={event=>setConfiguration({...configuration,supportEmail:event.target.value})} placeholder="support@company.com"/></label><label>Maximum meeting duration (minutes)<input type="number" min={15} max={1440} value={configuration.maxMeetingDurationMinutes} onChange={event=>setConfiguration({...configuration,maxMeetingDurationMinutes:Number(event.target.value)})}/></label><label>Recording retention (days)<input type="number" min={1} max={3650} value={configuration.recordingRetentionDays} onChange={event=>setConfiguration({...configuration,recordingRetentionDays:Number(event.target.value)})}/></label><button disabled={saving}>{saving?"Saving…":"Save configuration"}</button></form>}</section></main>;
}
