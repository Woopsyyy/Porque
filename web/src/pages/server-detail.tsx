import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Play, RotateCw, Square, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Skeleton, Spinner } from "@/components/ui/misc";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { StateBadge } from "@/components/state-badge";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { ConsoleView } from "@/components/console-view";
import { MetricsView } from "@/components/metrics-view";
import { BackupsView } from "@/components/backups-view";
import { TunnelView } from "@/components/tunnel-view";
import { SettingsView } from "@/components/settings-view";
import { ModsView } from "@/components/mods-view";

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
  const remove = useMutation({
    mutationFn: () => api.deleteServer(serverId),
    onSuccess: () => {
      toast.success("Server deleted");
      qc.invalidateQueries({ queryKey: ["servers"] });
      navigate("/");
    },
    onError: onErr,
  });

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
          <TabsTrigger value="tunnel">Tunnel</TabsTrigger>
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
        <TabsContent value="tunnel">
          <TunnelView server={server} />
        </TabsContent>
        <TabsContent value="settings">
          <SettingsView server={server} />
        </TabsContent>
      </Tabs>

      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title={`Delete “${server.name}”?`}
        description="This permanently removes the container, its data volume, and all backups. This cannot be undone."
        confirmLabel="Delete server"
        variant="danger"
        loading={remove.isPending}
        onConfirm={() => remove.mutate()}
      />
    </div>
  );
}
