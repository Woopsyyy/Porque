import { useEffect, useState } from "react";
import { Download, RefreshCw, X } from "lucide-react";
import { EventsOn, EventsOff, BrowserOpenURL } from "../../wailsjs/runtime";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/misc";

type UpdateState =
  | { kind: "idle" }
  | { kind: "downloading"; version: string }
  | { kind: "ready"; version: string }
  | { kind: "failed"; version: string; url: string };

// UpdateBanner listens for the Go auto-updater's events and surfaces a slim
// banner: downloading → ready (Restart) → failed (manual download).
export function UpdateBanner() {
  const [state, setState] = useState<UpdateState>({ kind: "idle" });
  const [restarting, setRestarting] = useState(false);

  useEffect(() => {
    EventsOn("update:available", (p: { version?: string }) =>
      setState({ kind: "downloading", version: p?.version ?? "" }),
    );
    EventsOn("update:ready", (p: { version?: string }) =>
      setState({ kind: "ready", version: p?.version ?? "" }),
    );
    EventsOn("update:failed", (p: { version?: string; url?: string }) =>
      setState({ kind: "failed", version: p?.version ?? "", url: p?.url ?? "" }),
    );
    return () => {
      EventsOff("update:available");
      EventsOff("update:ready");
      EventsOff("update:failed");
    };
  }, []);

  if (state.kind === "idle") return null;

  const restart = async () => {
    setRestarting(true);
    try {
      await api.restartApp();
    } catch {
      setRestarting(false);
    }
  };

  return (
    <div className="flex items-center justify-between gap-3 border-b border-gold/30 bg-gold/10 px-6 py-2 text-sm text-ink">
      {state.kind === "downloading" && (
        <span className="inline-flex items-center gap-2 text-muted">
          <Spinner className="h-3.5 w-3.5" />
          Downloading update {state.version}…
        </span>
      )}

      {state.kind === "ready" && (
        <>
          <span className="inline-flex items-center gap-2">
            <RefreshCw className="h-4 w-4 text-gold" />
            Porque {state.version} is ready — restart to apply.
          </span>
          <div className="flex items-center gap-2">
            <Button variant="primary" size="sm" onClick={restart} disabled={restarting}>
              {restarting ? <Spinner className="h-3.5 w-3.5" /> : <RefreshCw className="h-3.5 w-3.5" />}
              Restart now
            </Button>
            <button
              aria-label="Dismiss"
              className="text-faint hover:text-ink"
              onClick={() => setState({ kind: "idle" })}
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </>
      )}

      {state.kind === "failed" && (
        <>
          <span className="inline-flex items-center gap-2">
            <Download className="h-4 w-4 text-warn" />
            Porque {state.version} is available — automatic update couldn't be applied.
          </span>
          <div className="flex items-center gap-2">
            <Button
              variant="secondary"
              size="sm"
              onClick={() => state.url && BrowserOpenURL(state.url)}
            >
              <Download className="h-3.5 w-3.5" />
              Download
            </Button>
            <button
              aria-label="Dismiss"
              className="text-faint hover:text-ink"
              onClick={() => setState({ kind: "idle" })}
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </>
      )}
    </div>
  );
}
