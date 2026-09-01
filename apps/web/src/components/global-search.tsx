"use client";

import { useRouter } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";

type SearchResult={id:string;type:string;title:string;subtitle:string;href:string;updatedAt:string};
const labels:Record<string,string>={meeting:"Meetings",person:"People",chat:"Chat",room:"Rooms",drive:"Drive",calendar:"Calendar",recording:"Recordings"};

export function GlobalSearch({compact=false}:{compact?:boolean}){
  const router=useRouter(),root=useRef<HTMLDivElement>(null),input=useRef<HTMLInputElement>(null);
  const[open,setOpen]=useState(false),[query,setQuery]=useState(""),[results,setResults]=useState<SearchResult[]>([]),[loading,setLoading]=useState(false),[active,setActive]=useState(0),[error,setError]=useState("");
  useEffect(()=>{function shortcut(event:KeyboardEvent){if((event.metaKey||event.ctrlKey)&&event.key.toLowerCase()==="k"){event.preventDefault();setOpen(true);requestAnimationFrame(()=>input.current?.focus())}if(event.key==="Escape")setOpen(false)}function outside(event:MouseEvent){if(root.current&&!root.current.contains(event.target as Node))setOpen(false)}document.addEventListener("keydown",shortcut);document.addEventListener("mousedown",outside);return()=>{document.removeEventListener("keydown",shortcut);document.removeEventListener("mousedown",outside)}},[]);
  useEffect(()=>{const value=query.trim();if(value.length<2)return;const controller=new AbortController(),timer=window.setTimeout(()=>{setLoading(true);setError("");void fetch(`/api/v1/search?q=${encodeURIComponent(value)}`,{signal:controller.signal}).then(async response=>{const data=await response.json().catch(()=>({}));if(!response.ok)throw new Error(data?.error?.message??"Search is unavailable");setResults(data.results??[])}).catch(reason=>{if(reason?.name!=="AbortError")setError(reason instanceof Error?reason.message:"Search is unavailable")}).finally(()=>{if(!controller.signal.aborted)setLoading(false)})},220);return()=>{window.clearTimeout(timer);controller.abort()}},[query]);
  const grouped=useMemo(()=>results.reduce<Record<string,SearchResult[]>>((items,result)=>{(items[result.type]??=[]).push(result);return items},{}),[results]);
  function updateQuery(value:string){setQuery(value);setActive(0);if(value.trim().length<2){setResults([]);setLoading(false);setError("")}}
  function choose(result:SearchResult){setOpen(false);updateQuery("");router.push(result.href)}
  function keyboard(event:React.KeyboardEvent<HTMLInputElement>){if(event.key==="ArrowDown"){event.preventDefault();setActive(index=>Math.min(results.length-1,index+1))}else if(event.key==="ArrowUp"){event.preventDefault();setActive(index=>Math.max(0,index-1))}else if(event.key==="Enter"&&results[active]){event.preventDefault();choose(results[active])}}
  return <div ref={root} className={`global-search ${compact?"compact":""}`}>
    {compact?<button className="global-search-open" aria-label="Search workspace" onClick={()=>{setOpen(true);requestAnimationFrame(()=>input.current?.focus())}}><SearchIcon/></button>:<label className="global-search-field"><SearchIcon/><input ref={input} type="search" role="combobox" aria-expanded={open} aria-controls="global-search-results" aria-autocomplete="list" value={query} onFocus={()=>setOpen(true)} onChange={event=>{updateQuery(event.target.value);setOpen(true)}} onKeyDown={keyboard} placeholder="Search workspace…"/><kbd>⌘ K</kbd></label>}
    {open&&<div className="global-search-panel" role="dialog" aria-label="Search workspace">
      {compact&&<label className="global-search-field mobile"><SearchIcon/><input ref={input} type="search" role="combobox" aria-expanded="true" aria-controls="global-search-results" autoFocus value={query} onChange={event=>updateQuery(event.target.value)} onKeyDown={keyboard} placeholder="Search meetings, people, chat…"/><button aria-label="Close search" onClick={()=>setOpen(false)}>×</button></label>}
      <div id="global-search-results" role="listbox" className="global-search-results">
        {query.trim().length<2?<div className="global-search-state"><SearchIcon/><strong>Search your workspace</strong><span>Meetings, people, chat, rooms, files, events, and recordings.</span></div>:loading?<div className="global-search-state">Searching…</div>:error?<div className="global-search-state error">{error}</div>:results.length===0?<div className="global-search-state"><strong>No results found</strong><span>Try another name, code, or keyword.</span></div>:Object.entries(grouped).map(([type,items])=><section key={type}><h3>{labels[type]??type}</h3>{items.map(result=>{const index=results.indexOf(result);return <button key={`${result.type}-${result.id}`} role="option" aria-selected={index===active} className={index===active?"active":""} onMouseEnter={()=>setActive(index)} onClick={()=>choose(result)}><span className={`global-search-icon ${result.type}`}>{icon(result.type)}</span><span><strong>{result.title}</strong><small>{result.subtitle}</small></span><b>→</b></button>})}</section>)}
      </div>
      <footer><span><kbd>↑</kbd><kbd>↓</kbd> Navigate</span><span><kbd>↵</kbd> Open</span><span><kbd>esc</kbd> Close</span></footer>
    </div>}
  </div>
}

function SearchIcon(){return <svg aria-hidden="true" viewBox="0 0 24 24"><circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/></svg>}
function icon(type:string){return({meeting:"▣",person:"◉",chat:"▢",room:"⌘",drive:"◇",calendar:"□",recording:"▶"} as Record<string,string>)[type]??"•"}
