import { useState, useRef, useEffect } from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Terminal, RefreshCw, Search, ShieldCheck, AlertOctagon, Copy, Check } from "lucide-react";
import { api } from "@/lib/api";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

function formatTimeAgo(dateStr: string) {
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  if (diffMs < 0) return "just now"; // guard against small clock drifts
  
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMins / 60);

  if (diffMins < 1) return "just now";
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  return date.toLocaleString();
}

export default function LogsPage() {
  const [search, setSearch] = useState("");
  const [selectedServer, setSelectedServer] = useState<{ id: string; name: string } | null>(null);
  const [copied, setCopied] = useState(false);
  const consoleRef = useRef<HTMLDivElement>(null);

  const { data: logs, isLoading, refetch, isRefetching } = useQuery({
    queryKey: ["app-logs"],
    queryFn: api.listAppLogs,
    refetchInterval: 10000, // auto-refresh logs list every 10 seconds
  });

  const { data: consoleLogs, isLoading: isLoadingLogs } = useQuery({
    queryKey: ["server-console-logs", selectedServer?.id],
    queryFn: () => selectedServer ? api.getServerLogs(selectedServer.id) : Promise.resolve(""),
    enabled: !!selectedServer,
  });

  // Auto-scroll to bottom of logs on load/update
  useEffect(() => {
    if (consoleRef.current) {
      consoleRef.current.scrollTop = consoleRef.current.scrollHeight;
    }
  }, [consoleLogs]);

  const handleCopy = () => {
    if (consoleLogs) {
      navigator.clipboard.writeText(consoleLogs);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const filteredLogs = logs?.filter((log) => {
    const term = search.toLowerCase();
    return (
      log.server_name.toLowerCase().includes(term) ||
      log.message.toLowerCase().includes(term)
    );
  }) ?? [];

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="h1 flex items-center gap-2.5">
            <Terminal className="h-7 w-7 text-gold" />
            System Event Logs
          </h1>
          <p className="text-sm text-muted mt-1">
            Historical records of server crashes and runtime events over the last 24 hours. Click a log entry to view full server console logs.
          </p>
        </div>

        <button
          onClick={() => refetch()}
          disabled={isLoading || isRefetching}
          className="btn btn-secondary flex items-center gap-2 self-start sm:self-center"
        >
          <RefreshCw className={`h-4 w-4 ${isRefetching ? "animate-spin" : ""}`} />
          Refresh
        </button>
      </div>

      {/* Filter and Search Bar */}
      <div className="relative max-w-md">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-faint" />
        <input
          type="text"
          placeholder="Filter logs by server or message..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-full rounded-md border border-border bg-surface-2 py-2 pl-10 pr-4 text-sm text-ink placeholder-faint focus:border-gold focus:outline-none"
        />
      </div>

      {isLoading ? (
        <div className="flex h-48 items-center justify-center rounded-lg border border-border bg-surface/30">
          <RefreshCw className="h-7 w-7 animate-spin text-gold" />
        </div>
      ) : filteredLogs.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-border bg-surface/30 px-6 py-16 text-center">
          {search ? (
            <>
              <Search className="h-12 w-12 text-faint mb-4" />
              <h3 className="text-lg font-medium text-ink">No logs match search filter</h3>
              <p className="text-sm text-muted mt-1 max-w-sm">
                Try refining your search term or clear the input to view all events.
              </p>
            </>
          ) : (
            <>
              <div className="flex h-12 w-12 items-center justify-center rounded-full bg-running/10 text-running mb-4">
                <ShieldCheck className="h-6 w-6" />
              </div>
              <h3 className="text-lg font-medium text-ink">Everything running smoothly</h3>
              <p className="text-sm text-muted mt-1 max-w-sm">
                No server crashes or critical system events have been recorded in the last 24 hours.
              </p>
            </>
          )}
        </div>
      ) : (
        <div className="space-y-3">
          {filteredLogs.map((log) => (
            <div
              key={log.id}
              onClick={() => setSelectedServer({ id: log.server_id, name: log.server_name })}
              className="flex flex-col gap-4 rounded-lg border border-border bg-surface/50 p-4 transition-colors hover:bg-surface/80 sm:flex-row sm:items-start cursor-pointer select-none"
            >
              {/* Event Category Badge */}
              <div className="flex shrink-0 items-center gap-1.5 self-start rounded bg-danger/10 px-2 py-1 text-xs font-semibold text-danger">
                <AlertOctagon className="h-3.5 w-3.5" />
                CRASH
              </div>

              {/* Event Description */}
              <div className="min-w-0 flex-1 space-y-1">
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <span className="font-mono text-xs text-muted">
                    {formatTimeAgo(log.created_at)}
                  </span>
                  <span className="text-faint font-mono text-[0.68rem]">•</span>
                  <Link
                    to={`/servers/${log.server_id}`}
                    onClick={(e) => e.stopPropagation()}
                    className="font-medium text-gold hover:underline text-sm font-mono"
                  >
                    {log.server_name}
                  </Link>
                </div>
                <p className="text-sm font-mono text-ink bg-surface-2/65 rounded border border-border/40 p-2.5 whitespace-pre-wrap break-all">
                  {log.message}
                </p>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Full Server Console Logs Modal */}
      <Dialog open={!!selectedServer} onOpenChange={(open) => !open && setSelectedServer(null)}>
        <DialogContent className="max-w-4xl h-[80vh] flex flex-col">
          <DialogHeader className="flex flex-row items-center justify-between pr-8 mb-2">
            <div>
              <DialogTitle className="flex items-center gap-2">
                <Terminal className="h-5 w-5 text-gold" />
                Console Logs: {selectedServer?.name}
              </DialogTitle>
              <p className="text-xs text-muted mt-0.5">
                Inspect raw console output (`server.log`) to diagnose crash errors or misconfigurations.
              </p>
            </div>
            
            {consoleLogs && !isLoadingLogs && (
              <button
                onClick={handleCopy}
                className="btn btn-secondary flex items-center gap-1.5 py-1 px-2.5 text-xs select-none"
              >
                {copied ? (
                  <>
                    <Check className="h-3.5 w-3.5 text-success" />
                    Copied
                  </>
                ) : (
                  <>
                    <Copy className="h-3.5 w-3.5" />
                    Copy Logs
                  </>
                )}
              </button>
            )}
          </DialogHeader>

          <div 
            ref={consoleRef}
            className="flex-1 min-h-0 bg-zinc-950 rounded border border-border p-4 font-mono text-xs text-zinc-300 overflow-y-auto select-text whitespace-pre-wrap leading-relaxed"
          >
            {isLoadingLogs ? (
              <div className="flex h-full items-center justify-center">
                <RefreshCw className="h-6 w-6 animate-spin text-gold" />
              </div>
            ) : consoleLogs ? (
              consoleLogs
            ) : (
              <span className="text-faint">No logs recorded in server.log.</span>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
