import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, FolderX, Play, RotateCw, Square, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Skeleton, Spinner } from "@/components/ui/misc";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { StateBadge } from "@/components/state-badge";
import { ConsoleView } from "@/components/console-view";
import { MetricsView } from "@/components/metrics-view";
import { BackupsView } from "@/components/backups-view";
import { GeyserView } from "@/components/geyser-view";
import { SettingsView } from "@/components/settings-view";
import { ModsView } from "@/components/mods-view";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

const BUSY_STATES = ["starting", "stopping", "creating", "recovering"];

export default function ServerDetailPage() {
  const { id } = useParams<{ id: string }>();
  const serverId = id!;
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [confirmDelete, setConfirmDelete] = useState(false);

  const { data: server, isLoading } = useQuery({
    queryKey: ["server", serverId],
    queryFn: () => api.getServer(serverId),
    refetchInterval: 4000,
  });

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["server", serverId] });
    qc.invalidateQueries({ queryKey: ["servers"] });
  };

  const onErr = (e: unknown) =>
    toast.error(e instanceof ApiError ? e.message : "Action failed");

  const start = useMutation({
    mutationFn: () => api.startServer(serverId),
    onSuccess: () => {
      toast.success("Server started");
      invalidate();
    },
    onError: onErr,
  });
  const stop = useMutation({
    mutationFn: () => api.stopServer(serverId),
    onSuccess: () => {
      toast.success("Server stopped");
      invalidate();
    },
    onError: onErr,
  });
  const restart = useMutation({
    mutationFn: () => api.restartServer(serverId),
    onSuccess: () => {
      toast.success("Server restarted");
      invalidate();
    },
    onError: onErr,
  });

  const afterDelete = () => {
    qc.invalidateQueries({ queryKey: ["servers"] });
    navigate("/");
  };

  const removeRecord = useMutation({
    mutationFn: () => api.deleteServerRecord(serverId),
    onSuccess: () => {
      toast.success("Server removed from Porque — files kept on disk");
      afterDelete();
    },
    onError: onErr,
  });
  const removeAll = useMutation({
    mutationFn: () => api.deleteServer(serverId),
    onSuccess: () => {
      toast.success("Server and all files permanently deleted");
      afterDelete();
    },
    onError: onErr,
  });

  const deleting = removeRecord.isPending || removeAll.isPending;

  if (isLoading || !server) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-80 w-full" />
      </div>
    );
  }

  const running = server.state === "running";
  const busy = BUSY_STATES.includes(server.state);
  const actionPending = start.isPending || stop.isPending || restart.isPending;

  return (
    <div className="space-y-6">
      <Link
        to="/"
        className="inline-flex items-center gap-1.5 text-sm text-muted transition-colors hover:text-ink"
      >
        <ArrowLeft className="h-4 w-4" />
        All servers
      </Link>

      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <span className="eyebrow">
            {server.server_type} · {server.version}
            {server.loader_version ? ` · ${server.loader_version}` : ""}
          </span>
          <div className="mt-1.5 flex items-center gap-3">
            <h1 className="font-display text-3xl font-extrabold tracking-tight text-ink">
              {server.name}
            </h1>
            <StateBadge state={server.state} />
          </div>
          <p className="mt-1.5 font-mono text-xs text-faint">
            {server.cpu_cores} cores · {server.memory_mb} MB · {server.volume_name}
          </p>
        </div>

        <div className="flex items-center gap-2">
          {!running && (
            <Button
              variant="primary"
              onClick={() => start.mutate()}
              disabled={busy || actionPending}
            >
              {start.isPending ? <Spinner className="h-4 w-4" /> : <Play className="h-4 w-4" />}
              Start
            </Button>
          )}
          {running && (
            <Button variant="secondary" onClick={() => restart.mutate()} disabled={actionPending}>
              {restart.isPending ? <Spinner className="h-4 w-4" /> : <RotateCw className="h-4 w-4" />}
              Restart
            </Button>
          )}
          {running && (
            <Button variant="secondary" onClick={() => stop.mutate()} disabled={actionPending}>
              {stop.isPending ? <Spinner className="h-4 w-4" /> : <Square className="h-4 w-4" />}
              Stop
            </Button>
          )}
          <Button
            variant="danger"
            size="icon"
            title="Delete server"
            onClick={() => setConfirmDelete(true)}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </header>

      <Tabs defaultValue="console">
        <TabsList>
          <TabsTrigger value="console">Console</TabsTrigger>
          <TabsTrigger value="metrics">Metrics</TabsTrigger>
          <TabsTrigger value="mods">Mods</TabsTrigger>
          <TabsTrigger value="backups">Backups</TabsTrigger>
          <TabsTrigger value="geyser">Geyser</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent value="console">
          <ConsoleView serverId={serverId} running={running} />
        </TabsContent>
        <TabsContent value="metrics">
          <MetricsView server={server} />
        </TabsContent>
        <TabsContent value="mods">
          <ModsView server={server} />
        </TabsContent>
        <TabsContent value="backups">
          <BackupsView server={server} />
        </TabsContent>
        <TabsContent value="geyser">
          <GeyserView server={server} />
        </TabsContent>
        <TabsContent value="settings">
          <SettingsView server={server} />
        </TabsContent>
      </Tabs>

      {/* ─── Two-option delete dialog ─────────────────────────────────── */}
      <Dialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-danger">
              <Trash2 className="h-5 w-5" />
              Delete "{server.name}"?
            </DialogTitle>
            <DialogDescription>
              Choose how you want to remove this server. This cannot be undone.
            </DialogDescription>
          </DialogHeader>

          <div className="mt-1 flex flex-col gap-3">
            {/* Option 1 — record only */}
            <button
              id="delete-record-only"
              disabled={deleting}
              onClick={() => removeRecord.mutate()}
              className="group flex items-start gap-4 rounded-xl border border-border bg-surface/60 p-4 text-left transition-all hover:border-warn/50 hover:bg-warn/5 disabled:cursor-not-allowed disabled:opacity-50"
            >
              <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-warn/10 text-warn transition-colors group-hover:bg-warn/20">
                {removeRecord.isPending ? (
                  <Spinner className="h-4 w-4" />
                ) : (
                  <Trash2 className="h-4 w-4" />
                )}
              </div>
              <div>
                <p className="font-semibold text-ink">Remove from Porque only</p>
                <p className="mt-0.5 text-sm text-muted">
                  Unlinks the server from the dashboard. Your server files at{" "}
                  <span className="font-mono text-xs text-faint">{server.volume_name}</span>{" "}
                  are <span className="font-medium text-ink">kept on disk</span>.
                </p>
              </div>
            </button>

            {/* Option 2 — record + files */}
            <button
              id="delete-record-and-files"
              disabled={deleting}
              onClick={() => removeAll.mutate()}
              className="group flex items-start gap-4 rounded-xl border border-border bg-surface/60 p-4 text-left transition-all hover:border-danger/50 hover:bg-danger/5 disabled:cursor-not-allowed disabled:opacity-50"
            >
              <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-danger/10 text-danger transition-colors group-hover:bg-danger/20">
                {removeAll.isPending ? (
                  <Spinner className="h-4 w-4" />
                ) : (
                  <FolderX className="h-4 w-4" />
                )}
              </div>
              <div>
                <p className="font-semibold text-danger">Delete from Porque + directory</p>
                <p className="mt-0.5 text-sm text-muted">
                  Permanently deletes the server record <span className="font-medium text-ink">and</span>{" "}
                  all files at{" "}
                  <span className="font-mono text-xs text-faint">{server.volume_name}</span>.{" "}
                  <span className="text-danger font-medium">Irreversible.</span>
                </p>
              </div>
            </button>

            <Button
              variant="ghost"
              className="mt-1"
              onClick={() => setConfirmDelete(false)}
              disabled={deleting}
            >
              Cancel
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
