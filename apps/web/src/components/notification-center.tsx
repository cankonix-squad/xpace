"use client";

import { useRouter } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";

type NotificationItem = {
  id:string;
  type:string;
  conversationId?:string;
  messageId?:string;
  actorName?:string;
  payload:{preview?:string;emoji?:string};
  readAt?:string;
  createdAt:string;
};

export function NotificationCenter() {
  const router=useRouter();
  const rootRef=useRef<HTMLDivElement>(null);
  const[items,setItems]=useState<NotificationItem[]>([]),[unread,setUnread]=useState(0),[open,setOpen]=useState(false),[loading,setLoading]=useState(true),[error,setError]=useState("");
  const load=useCallback(async()=>{const response=await fetch("/api/v1/notifications?unreadOnly=false"),data=await response.json().catch(()=>({}));if(!response.ok)throw new Error(data?.error?.message??"Could not load notifications");setItems(data.notifications??[]);setUnread(data.unreadCount??0);setError("");setLoading(false)},[]);

  useEffect(()=>{let active=true;const refresh=()=>{void load().catch(reason=>{if(active){setLoading(false);setError(reason instanceof Error?reason.message:"Could not load notifications")}})};refresh();const timer=window.setInterval(refresh,30000);window.addEventListener("focus",refresh);return()=>{active=false;window.clearInterval(timer);window.removeEventListener("focus",refresh)}},[load]);
  useEffect(()=>{if(!open)return;const closeOutside=(event:PointerEvent)=>{if(event.target instanceof Node&&!rootRef.current?.contains(event.target))setOpen(false)};const closeWithEscape=(event:KeyboardEvent)=>{if(event.key==="Escape")setOpen(false)};document.addEventListener("pointerdown",closeOutside,true);document.addEventListener("keydown",closeWithEscape);return()=>{document.removeEventListener("pointerdown",closeOutside,true);document.removeEventListener("keydown",closeWithEscape)}},[open]);

  async function markRead(item:NotificationItem){if(!item.readAt){const response=await fetch(`/api/v1/notifications/${item.id}/read`,{method:"POST"});if(response.ok){setItems(current=>current.map(value=>value.id===item.id?{...value,readAt:new Date().toISOString()}:value));setUnread(value=>Math.max(0,value-1))}}setOpen(false);if(item.conversationId){window.dispatchEvent(new CustomEvent("xpace-notification-open",{detail:{conversationId:item.conversationId,messageId:item.messageId??""}}));router.push(`/chat?conversationId=${encodeURIComponent(item.conversationId)}${item.messageId?`&messageId=${encodeURIComponent(item.messageId)}`:""}`)}}
  async function markAll(){const response=await fetch("/api/v1/notifications/read-all",{method:"POST"});if(!response.ok){setError("Could not mark notifications as read");return}const now=new Date().toISOString();setItems(current=>current.map(item=>({...item,readAt:item.readAt??now})));setUnread(0)}

  return <div className="global-notifications" ref={rootRef}>
    <button className="notification-trigger" type="button" aria-label={unread?`Notifications, ${unread} unread`:"Notifications"} aria-expanded={open} onClick={()=>setOpen(value=>!value)}><BellIcon/>{unread>0&&<b>{unread>99?"99+":unread}</b>}</button>
    {open&&<section className="notification-dropdown" aria-label="Notifications"><header><div><strong>Notifications</strong><span>{unread?`${unread} unread`:"You’re all caught up"}</span></div><button disabled={unread===0} onClick={()=>void markAll()}>Mark all read</button></header>{error&&<p className="notification-error" role="alert">{error}</p>}<div className="notification-list">{loading?<p className="notification-empty">Loading notifications…</p>:items.length===0?<p className="notification-empty">No notifications yet.</p>:items.map(item=><button key={item.id} className={item.readAt?"read":"unread"} onClick={()=>void markRead(item)}><i/><span><strong>{notificationTitle(item)}</strong><small>{item.payload.preview||item.payload.emoji||"New activity"}</small><time>{formatTime(item.createdAt)}</time></span></button>)}</div></section>}
  </div>;
}

function BellIcon(){return <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4"/></svg>}
function notificationTitle(item:NotificationItem){const actor=item.actorName||"A teammate";if(item.type==="CHAT_MENTION")return`${actor} mentioned you`;if(item.type==="CHAT_REPLY")return`${actor} replied to you`;if(item.type==="CHAT_REACTION")return`${actor} reacted to your message`;return`${actor} sent an update`}
function formatTime(value:string){const date=new Date(value),now=new Date(),sameDay=date.toDateString()===now.toDateString();return new Intl.DateTimeFormat("en",sameDay?{hour:"2-digit",minute:"2-digit"}:{month:"short",day:"numeric"}).format(date)}
