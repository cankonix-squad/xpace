"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import styles from "./chat.module.css";

type Conversation = { id: string; type: "DIRECT" | "CHANNEL"; name: string; createdAt: string; memberCount: number; unreadCount: number; onlineCount: number };
type DirectoryUser = { id: string; username: string; displayName: string };
type Attachment = { id: string; originalName: string; sizeBytes: number };
type Message = { id: string; conversationId: string; body: string; senderId: string; senderName: string; createdAt: string; parentId?: string; editedAt?: string; deletedAt?: string; pinnedAt?: string; reactionCount?: number; attachments?: Attachment[] };
type ComposeMode = "DIRECT" | "CHANNEL" | "";

export default function ChatPage() {
  const router = useRouter();
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [directory, setDirectory] = useState<DirectoryUser[]>([]);
  const [sessionUserID, setSessionUserID] = useState("");
  const [selected, setSelected] = useState("");
  const [messages, setMessages] = useState<Message[]>([]);
  const [body, setBody] = useState("");
  const [attachment, setAttachment] = useState<File | null>(null);
  const [emojiOpen, setEmojiOpen] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [replyTo, setReplyTo] = useState<Message | null>(null);
  const [search, setSearch] = useState("");
  const [searchResults, setSearchResults] = useState<Message[]>([]);
  const [composeMode, setComposeMode] = useState<ComposeMode>("");
  const [channelName, setChannelName] = useState("");
  const [memberSearch, setMemberSearch] = useState("");
  const [memberIDs, setMemberIDs] = useState<string[]>([]);
  const [conversationMenu, setConversationMenu] = useState("");

  const loadConversations = useCallback(async () => {
    const response = await fetch("/api/v1/chat/conversations");
    if (response.status === 401) { router.replace("/login"); return; }
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data?.error?.message ?? "Could not load conversations");
    const items: Conversation[] = data.conversations ?? [];
    const requested = new URLSearchParams(window.location.search).get("conversationId") ?? "";
    setConversations(items);
    setSelected(current => current || (items.some(item => item.id === requested) ? requested : items[0]?.id || ""));
  }, [router]);

  const loadMessages = useCallback(async (id: string) => {
    if (!id) { setMessages([]); return; }
    const response = await fetch(`/api/v1/chat/conversations/${encodeURIComponent(id)}/messages`);
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data?.error?.message ?? "Could not load messages");
    setMessages((data.messages ?? []).reverse());
  }, []);

  useEffect(() => {
    let mounted = true;
    const run = async () => {
      try {
        const [directoryResponse, sessionResponse] = await Promise.all([fetch("/api/v1/directory/users"), fetch("/api/v1/auth/me")]);
        const [directoryData, sessionData] = await Promise.all([directoryResponse.json().catch(() => ({})), sessionResponse.json().catch(() => ({}))]);
        if (!directoryResponse.ok) throw new Error(directoryData?.error?.message ?? "Could not load workspace users");
        if (mounted) {
          setDirectory(directoryData.users ?? []);
          setSessionUserID(sessionData?.user?.id ?? "");
        }
        await loadConversations();
      } catch (reason) {
        if (mounted) setError(reason instanceof Error ? reason.message : "Could not load chat");
      } finally {
        if (mounted) setLoading(false);
      }
    };
    void run();
    return () => { mounted = false; };
  }, [loadConversations]);

  useEffect(() => {
    const open = (event: Event) => {
      const detail = (event as CustomEvent<{ conversationId?: string }>).detail;
      if (detail?.conversationId) setSelected(detail.conversationId);
    };
    window.addEventListener("xpace-notification-open", open);
    return () => window.removeEventListener("xpace-notification-open", open);
  }, []);

  useEffect(() => {
    let mounted = true;
    const run = async () => {
      try {
        await loadMessages(selected);
        if (!selected) return;
        await fetch(`/api/v1/chat/conversations/${encodeURIComponent(selected)}/read`, { method: "POST" });
        await fetch(`/api/v1/chat/conversations/${encodeURIComponent(selected)}/presence`, { method: "POST" });
      } catch (reason) {
        if (mounted) setError(reason instanceof Error ? reason.message : "Could not load messages");
      }
    };
    void run();
    if (!selected) return () => { mounted = false; };
    const stream = new EventSource(`/api/v1/chat/conversations/${encodeURIComponent(selected)}/events`);
    const onMessage = (event: Event) => {
      const message = JSON.parse((event as MessageEvent).data) as Message;
      setMessages(items => items.some(item => item.id === message.id) ? items : [...items, message]);
      void fetch(`/api/v1/chat/conversations/${encodeURIComponent(selected)}/read`, { method: "POST" });
    };
    const onPresence = () => setConversations(items => items.map(item => item.id === selected ? { ...item, onlineCount: Math.max(1, item.onlineCount) } : item));
    stream.addEventListener("message", onMessage);
    stream.addEventListener("presence", onPresence);
    const heartbeat = window.setInterval(() => void fetch(`/api/v1/chat/conversations/${encodeURIComponent(selected)}/presence`, { method: "POST" }), 30000);
    return () => {
      mounted = false;
      stream.removeEventListener("message", onMessage);
      stream.removeEventListener("presence", onPresence);
      stream.close();
      window.clearInterval(heartbeat);
    };
  }, [loadMessages, selected]);

  const active = useMemo(() => conversations.find(item => item.id === selected), [conversations, selected]);
  const availableUsers = useMemo(() => directory.filter(user => user.id !== sessionUserID && `${user.displayName} ${user.username}`.toLowerCase().includes(memberSearch.toLowerCase())), [directory, memberSearch, sessionUserID]);

  useEffect(() => {
    if (!conversationMenu) return;
    const close = () => setConversationMenu("");
    window.addEventListener("click", close);
    return () => window.removeEventListener("click", close);
  }, [conversationMenu]);

  function openComposer(mode: Exclude<ComposeMode, "">) {
    setComposeMode(mode);
    setChannelName("");
    setMemberSearch("");
    setMemberIDs([]);
    setError("");
  }

  function closeComposer() {
    if (creating) return;
    setComposeMode("");
  }

  function toggleMember(id: string) {
    if (composeMode === "DIRECT") { setMemberIDs([id]); return; }
    setMemberIDs(items => items.includes(id) ? items.filter(item => item !== id) : [...items, id]);
  }

  async function createConversation(event: FormEvent) {
    event.preventDefault();
    if (!composeMode || memberIDs.length === 0 || (composeMode === "CHANNEL" && channelName.trim().length < 2)) return;
    setCreating(true);
    setError("");
    const response = await fetch("/api/v1/chat/conversations", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ type: composeMode, name: composeMode === "CHANNEL" ? channelName.trim() : "", memberIds: memberIDs }),
    });
    const data = await response.json().catch(() => ({}));
    setCreating(false);
    if (!response.ok) { setError(data?.error?.message ?? "Could not create conversation"); return; }
    const conversation = data.conversation as Conversation;
    setConversations(items => [conversation, ...items.filter(item => item.id !== conversation.id)]);
    setSelected(conversation.id);
    setComposeMode("");
  }

  async function sendMessage(event: FormEvent) {
    event.preventDefault();
    if (!selected || (!body.trim() && !attachment)) return;
    setError("");
    const messageBody = body.trim() || (attachment ? `📎 ${attachment.name}` : "");
    const response = await fetch(`/api/v1/chat/conversations/${encodeURIComponent(selected)}/messages`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ body: messageBody, parentId: replyTo?.id ?? "" }) });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) { setError(data?.error?.message ?? "Could not send message"); return; }
    let sent = data.message as Message;
    if (attachment) {
      const form = new FormData();
      form.append("file", attachment);
      const upload = await fetch(`/api/v1/chat/conversations/${encodeURIComponent(selected)}/messages/${sent.id}/attachments`, { method: "POST", body: form });
      const uploadData = await upload.json().catch(() => ({}));
      if (!upload.ok) { setError(uploadData?.error?.message ?? "Could not upload attachment"); return; }
      sent = { ...sent, attachments: [uploadData.attachment] };
    }
    setMessages(items => [...items, sent]);
    setBody("");
    setAttachment(null);
    setEmojiOpen(false);
    setReplyTo(null);
  }

  async function react(message: Message) { const response = await fetch(`/api/v1/chat/conversations/${encodeURIComponent(selected)}/messages/${message.id}/reactions`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ emoji: "👍" }) }); if (!response.ok) setError("Could not add reaction"); }
  async function pin(message: Message) { const response = await fetch(`/api/v1/chat/conversations/${encodeURIComponent(selected)}/messages/${message.id}/pin`, { method: message.pinnedAt ? "DELETE" : "POST" }); if (!response.ok) setError("Could not update pin"); }
  async function remove(message: Message) { const response = await fetch(`/api/v1/chat/conversations/${encodeURIComponent(selected)}/messages/${message.id}`, { method: "DELETE" }); if (!response.ok) setError("Could not delete message"); }
  async function edit(message: Message) { const value = window.prompt("Edit message", message.body); if (value === null || value.trim() === message.body) return; const response = await fetch(`/api/v1/chat/conversations/${encodeURIComponent(selected)}/messages/${message.id}`, { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ body: value }) }); if (!response.ok) setError("Could not edit message"); }
  async function searchMessages(value: string) { setSearch(value); if (value.trim().length < 2) { setSearchResults([]); return; } const response = await fetch(`/api/v1/chat/search?q=${encodeURIComponent(value)}${selected ? `&conversationId=${encodeURIComponent(selected)}` : ""}`); const data = await response.json().catch(() => ({})); if (response.ok) setSearchResults(data.messages ?? []); }
  async function openAttachment(message: Message, item: Attachment) { const response = await fetch(`/api/v1/chat/conversations/${encodeURIComponent(selected)}/messages/${message.id}/attachments/${item.id}/download`); const data = await response.json().catch(() => ({})); if (response.ok && data.url) window.open(data.url, "_blank", "noopener,noreferrer"); else setError("Could not open attachment"); }
  async function clearConversation(item: Conversation) {
    setConversationMenu("");
    if (!window.confirm(`Clear all messages in “${item.name || "Direct message"}” for your account? Other members will keep their messages.`)) return;
    const response = await fetch(`/api/v1/chat/conversations/${encodeURIComponent(item.id)}/clear`, { method: "POST" });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) { setError(data?.error?.message ?? "Could not clear chat"); return; }
    if (selected === item.id) { setMessages([]); setReplyTo(null); }
    setSearch("");
    setSearchResults([]);
    setError("");
  }
  async function deleteConversation(item: Conversation) {
    setConversationMenu("");
    if (!window.confirm(`Delete “${item.name || "Direct message"}” from your chat list? This only affects your account and new messages can make it appear again.`)) return;
    const response = await fetch(`/api/v1/chat/conversations/${encodeURIComponent(item.id)}`, { method: "DELETE" });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) { setError(data?.error?.message ?? "Could not delete conversation"); return; }
    const remaining = conversations.filter(conversation => conversation.id !== item.id);
    setConversations(remaining);
    if (selected === item.id) { setSelected(remaining[0]?.id ?? ""); setMessages([]); setReplyTo(null); }
    setSearch("");
    setSearchResults([]);
    setError("");
  }

  return <main className={styles.page}>
    <header className={styles.header}><div><p className={styles.kicker}>XSPACE COLLABORATION</p><h1>Chat</h1><span>Secure tenant conversations for your team.</span></div><div className={styles.headerActions}><button className={styles.back} onClick={() => router.push("/")}>← Workspace</button></div></header>
    {error && <p className={styles.error} role="alert">{error}</p>}
    <section className={styles.chat}>
      <aside className={styles.sidebar}>
        <div className={styles.sidebarHead}><div><h2>Conversations</h2><span>{conversations.length} available</span></div><button aria-label="Refresh conversations" onClick={() => void loadConversations()}>↻</button></div>
        <div className={styles.createActions}><button onClick={() => openComposer("DIRECT")}>＋ New message</button><button onClick={() => openComposer("CHANNEL")}>＋ Channel</button></div>
        <input className={styles.search} value={search} onChange={event => void searchMessages(event.target.value)} placeholder="Search messages…" aria-label="Search messages" />
        {searchResults.length > 0 && <div className={styles.searchResults}>{searchResults.map(result => <button key={result.id} onClick={() => { setSelected(result.conversationId); setSearch(""); setSearchResults([]); }}><strong>{result.senderName}</strong><span>{result.body}</span></button>)}</div>}
        {loading ? <p className={styles.empty}>Loading conversations…</p> : conversations.length === 0 ? <p className={styles.empty}>Start a direct message or create a channel.</p> : <div className={styles.conversationList}>{conversations.map(item => <div key={item.id} className={`${styles.conversationItem} ${item.id === selected ? styles.selected : ""}`}><button className={styles.conversationSelect} onClick={() => setSelected(item.id)}><span className={styles.avatar}>{(item.name || "Direct").slice(0, 1).toUpperCase()}</span><span><strong>{item.name || "Direct message"}</strong><small>{item.type === "CHANNEL" ? `${item.memberCount} members` : "Direct message"}</small></span>{item.unreadCount > 0 && <b className={styles.unread}>{item.unreadCount > 99 ? "99+" : item.unreadCount}</b>}</button><button className={styles.conversationMenuButton} aria-label={`Chat actions for ${item.name || "Direct message"}`} aria-haspopup="menu" aria-expanded={conversationMenu === item.id} onClick={event => { event.stopPropagation(); setConversationMenu(current => current === item.id ? "" : item.id); }}>⋮</button>{conversationMenu === item.id && <div className={styles.conversationMenu} role="menu" onClick={event => event.stopPropagation()}><button role="menuitem" onClick={() => void clearConversation(item)}>Clear chat</button><button className={styles.dangerAction} role="menuitem" onClick={() => void deleteConversation(item)}>Delete conversation</button></div>}</div>)}</div>}
      </aside>
      <section className={styles.thread}>
        <header className={styles.threadHead}><div><h2>{active?.name || "Select a conversation"}</h2><span>{active ? `${active.memberCount} members · ${active.onlineCount} online · ${active.type.toLowerCase()}` : "Choose a conversation from the left"}</span></div></header>
        <div className={styles.messages}>{active ? messages.length ? messages.map(message => { const mine = message.senderId === sessionUserID; return <article key={message.id} className={mine ? styles.mine : styles.theirs}>{!mine && <span className={styles.messageAvatar}>{message.senderName.slice(0, 1).toUpperCase()}</span>}<div className={styles.messageGroup}><div className={styles.bubble}>{!mine && <strong className={styles.sender}>{message.senderName}</strong>}{message.parentId && <small className={styles.replyLabel}>↳ Thread reply</small>}<p className={message.deletedAt ? styles.deleted : ""}>{message.body}</p>{message.attachments?.map(item => <button className={styles.attachment} key={item.id} onClick={() => void openAttachment(message, item)}>📎 {item.originalName}</button>)}<div className={styles.bubbleMeta}>{message.editedAt && !message.deletedAt && <span>edited</span>}<time>{new Intl.DateTimeFormat("en", { hour: "2-digit", minute: "2-digit" }).format(new Date(message.createdAt))}</time></div></div><div className={styles.messageActions}><button onClick={() => setReplyTo(message)}>Reply</button><button onClick={() => void react(message)}>👍 {message.reactionCount || ""}</button><button onClick={() => void pin(message)}>{message.pinnedAt ? "Unpin" : "Pin"}</button>{mine && <><button onClick={() => void edit(message)}>Edit</button><button onClick={() => void remove(message)}>Delete</button></>}</div></div></article>}) : <p className={styles.empty}>No messages yet. Start the conversation.</p> : <p className={styles.empty}>Select a conversation to view messages.</p>}</div>
        {replyTo && <div className={styles.replying}>Replying to {replyTo.senderName}<button type="button" onClick={() => setReplyTo(null)}>×</button></div>}
        <form className={styles.composer} onSubmit={sendMessage}><div className={styles.composerTools}><button className={styles.emojiButton} type="button" title="Add emoji" aria-label="Add emoji" aria-expanded={emojiOpen} onClick={() => setEmojiOpen(value => !value)} disabled={!active}>☺</button>{emojiOpen && <div className={styles.emojiPicker} role="menu" aria-label="Choose an emoji">{["😀","😂","😍","👍","🙏","🎉","❤️","🔥","✅","👋","😮","😢"].map(emoji => <button key={emoji} type="button" role="menuitem" onClick={() => { setBody(value => value + emoji); setEmojiOpen(false); }}>{emoji}</button>)}</div>}<label className={styles.attachButton} title="Attach file" aria-label="Attach file">📎<input type="file" onChange={event => setAttachment(event.target.files?.[0] ?? null)} disabled={!active} /></label></div><input value={body} onChange={event => setBody(event.target.value)} placeholder={attachment?.name || (active ? "Type a message" : "Select a conversation first")} disabled={!active} maxLength={4000} /><button disabled={!active || (!body.trim() && !attachment)}>Send</button></form>
      </section>
    </section>
    {composeMode && <div className={styles.modalBackdrop} onMouseDown={event => { if (event.currentTarget === event.target) closeComposer(); }}>
      <form className={styles.modal} onSubmit={createConversation} role="dialog" aria-modal="true" aria-labelledby="conversation-title">
        <header><div><span>{composeMode === "DIRECT" ? "DIRECT MESSAGE" : "NEW CHANNEL"}</span><h2 id="conversation-title">{composeMode === "DIRECT" ? "Start a conversation" : "Create a team channel"}</h2></div><button type="button" aria-label="Close" onClick={closeComposer}>×</button></header>
        {composeMode === "CHANNEL" && <label className={styles.field}>Channel name<input autoFocus value={channelName} onChange={event => setChannelName(event.target.value)} placeholder="e.g. Product team" minLength={2} maxLength={120} required /></label>}
        <label className={styles.field}>{composeMode === "DIRECT" ? "Choose one workspace user" : "Choose channel members"}<input autoFocus={composeMode === "DIRECT"} value={memberSearch} onChange={event => setMemberSearch(event.target.value)} placeholder="Search by name or username…" /></label>
        <div className={styles.userList}>{availableUsers.length === 0 ? <p>No active workspace users found.</p> : availableUsers.map(user => <button type="button" key={user.id} className={memberIDs.includes(user.id) ? styles.userSelected : ""} onClick={() => toggleMember(user.id)}><span className={styles.avatar}>{user.displayName.slice(0, 1).toUpperCase()}</span><span><strong>{user.displayName}</strong><small>@{user.username}</small></span><b>{memberIDs.includes(user.id) ? "✓" : composeMode === "DIRECT" ? "Chat" : "Add"}</b></button>)}</div>
        <footer><span>{memberIDs.length} selected</span><div><button type="button" className={styles.cancel} onClick={closeComposer}>Cancel</button><button className={styles.start} disabled={creating || memberIDs.length === 0 || (composeMode === "CHANNEL" && channelName.trim().length < 2)}>{creating ? "Starting…" : composeMode === "DIRECT" ? "Start chat" : "Create channel"}</button></div></footer>
      </form>
    </div>}
  </main>;
}
