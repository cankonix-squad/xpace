"use client";

import { useEffect } from "react";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    void fetch("/api/v1/errors/client", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        message: error.message || "Unexpected UI error",
        digest: error.digest || "",
        path: window.location.pathname,
      }),
    });
  }, [error]);
  return (
    <main className="csp-error-page">
      <section>
        <p className="csp-error-kicker">
          XSPACE RECOVERY
        </p>
        <h1>Something went wrong</h1>
        <p className="csp-error-copy">
          The incident was recorded. You can safely retry this screen.
        </p>
        <button className="csp-error-action" onClick={reset}>
          Try again
        </button>
      </section>
    </main>
  );
}
