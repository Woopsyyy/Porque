import { Link } from "react-router-dom";
import { ArrowRight, Cpu, MemoryStick, Play, RotateCw, Square } from "lucide-react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, ApiError, type Server } from "@/lib/api";
import { StateBadge } from "@/components/state-badge";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";

export function ServerCard({ server }: { server: Server }) {
  const qc = useQueryClient();

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["server", server.id] });
    qc.invalidateQueries({ queryKey: ["servers"] });
  };

  const onErr = (e: unknown) =>
    toast.error(e instanceof ApiError ? e.message : "Action failed");

  const start = useMutation({
    mutationFn: () => api.startServer(server.id),
    onSuccess: () => {
      toast.success(`Server "${server.name}" starting`);
      invalidate();
    },
    onError: onErr,
  });

  const stop = useMutation({
    mutationFn: () => api.stopServer(server.id),
    onSuccess: () => {
      toast.success(`Server "${server.name}" stopping`);
      invalidate();
    },
    onError: onErr,
  });

  const restart = useMutation({
    mutationFn: () => api.restartServer(server.id),
    onSuccess: () => {
      toast.success(`Server "${server.name}" restarting`);
      invalidate();
    },
    onError: onErr,
  });

  const running = server.state === "running";
  const starting = server.state === "starting";
  const showStart = !running && !starting && server.state !== "stopping";
  const showStop = running || starting;
  const showRestart = running;
  const actionPending = start.isPending || stop.isPending || restart.isPending;

  return (
    <ContextMenu>
      <ContextMenuTrigger>
        <Link
          to={`/servers/${server.id}`}
          className="panel group relative block overflow-hidden p-5 transition-all duration-200 hover:-translate-y-0.5 hover:border-gold/40 hover:shadow-glow"
        >
          {/* corner sheen on hover */}
          <div
            aria-hidden
            className="pointer-events-none absolute -right-16 -top-16 h-32 w-32 rounded-full bg-gold/10 opacity-0 blur-2xl transition-opacity duration-300 group-hover:opacity-100"
          />
          <div className="flex items-start justify-between">
            <span className="eyebrow">
              {server.server_type} · {server.version}
            </span>
            <StateBadge state={server.state} />
          </div>

          <h3 className="mt-3 font-display text-xl font-bold tracking-tight text-ink">{server.name}</h3>

          <div className="mt-4 flex items-center gap-4 font-mono text-xs text-muted">
            <span className="inline-flex items-center gap-1.5">
              <Cpu className="h-3.5 w-3.5 text-faint" />
              {server.cpu_cores} cores
            </span>
            <span className="inline-flex items-center gap-1.5">
              <MemoryStick className="h-3.5 w-3.5 text-faint" />
              {server.memory_mb} MB
            </span>
          </div>

          <div className="mt-4 flex items-center justify-end text-sm font-medium text-muted transition-colors group-hover:text-gold">
            Open
            <ArrowRight className="ml-1 h-4 w-4 transition-transform group-hover:translate-x-0.5" />
          </div>
        </Link>
      </ContextMenuTrigger>

      <ContextMenuContent className="w-52">
        <ContextMenuLabel className="font-semibold text-xs">
          {server.name} ({server.server_type})
        </ContextMenuLabel>
        <ContextMenuSeparator />

        {showStart && (
          <ContextMenuItem
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              start.mutate();
            }}
            disabled={actionPending}
          >
            <Play className="h-4 w-4 text-emerald-500" />
            Start
          </ContextMenuItem>
        )}

        {showStop && (
          <ContextMenuItem
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              stop.mutate();
            }}
            disabled={actionPending}
          >
            <Square className="h-4 w-4 text-rose-500" />
            Stop
          </ContextMenuItem>
        )}

        {showRestart && (
          <ContextMenuItem
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              restart.mutate();
            }}
            disabled={actionPending}
          >
            <RotateCw className="h-4 w-4 text-amber-500" />
            Restart
          </ContextMenuItem>
        )}

        {server.state === "stopping" && (
          <ContextMenuItem disabled>
            <span className="text-muted text-xs">Stopping...</span>
          </ContextMenuItem>
        )}
      </ContextMenuContent>
    </ContextMenu>
  );
}

