"use client";

import { useEffect } from "react";

export default function GlobalError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => {
    void fetch("/api/v1/errors/client", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message: error.message || "Unexpected UI error", digest: error.digest || "", path: window.location.pathname }),
    });
  }, [error]);
  return <main style={{minHeight:"100vh",display:"grid",placeItems:"center",padding:24,fontFamily:"Helvetica,Arial,sans-serif",background:"#080b09",color:"#eef2ee"}}><section><p style={{color:"#a3e635",fontSize:12,letterSpacing:2}}>XSPACE RECOVERY</p><h1>Something went wrong</h1><p style={{fontSize:14,color:"#9da99e"}}>The incident was recorded. You can safely retry this screen.</p><button onClick={reset} style={{minHeight:44,padding:"0 18px",border:0,borderRadius:8,background:"#a3e635",color:"#142006",fontSize:14,fontWeight:700}}>Try again</button></section></main>;
}
