import { useState, useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Clock,
  RotateCcw,
  ShieldAlert,
  ShieldCheck,
  Trash2,
  Database,
  CalendarDays,
} from "lucide-react";
import { toast } from "sonner";
import { api, ApiError, type Backup, type Server } from "@/lib/api";
import { formatBytes, formatRelative, shortHash } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton, Spinner } from "@/components/ui/misc";
import { ConfirmDialog } from "@/components/confirm-dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";

export function BackupsView({ server }: { server: Server }) {
  const qc = useQueryClient();
  const [restoreId, setRestoreId] = useState<string | null>(null);
  const [deleteId, setDeleteId] = useState<string | null>(null);

  // Scheduled backup form states
  const [enabled, setEnabled] = useState(server.backup_enabled);
  const [intervalValue, setIntervalValue] = useState(server.backup_interval_value || 6);
  const [intervalUnit, setIntervalUnit] = useState(server.backup_interval_unit || "hour");
  const [keep, setKeep] = useState(server.backup_keep || 5);

  // Sync form states with server changes
  useEffect(() => {
    setEnabled(server.backup_enabled);
    setIntervalValue(server.backup_interval_value || 6);
    setIntervalUnit(server.backup_interval_unit || "hour");
    setKeep(server.backup_keep || 5);
  }, [server]);

  const { data: backups, isLoading } = useQuery({
    queryKey: ["backups", server.id],
    queryFn: () => api.listBackups(server.id),
  });

  const create = useMutation({
    mutationFn: () => api.createBackup(server.id),
    onSuccess: () => {
      toast.success("Backup completed successfully");
      qc.invalidateQueries({ queryKey: ["backups", server.id] });
      qc.invalidateQueries({ queryKey: ["server", server.id] });
      qc.invalidateQueries({ queryKey: ["servers"] });
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Backup failed"),
  });

  const saveSchedule = useMutation({
    mutationFn: () =>
      api.updateBackupSchedule(server.id, {
        enabled,
        interval_value: Number(intervalValue),
        interval_unit: intervalUnit,
        keep: Number(keep),
      }),
    onSuccess: () => {
      toast.success("Backup schedule updated");
      qc.invalidateQueries({ queryKey: ["server", server.id] });
      qc.invalidateQueries({ queryKey: ["servers"] });
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Could not save schedule"),
  });

  const restore = useMutation({
    mutationFn: (id: string) => api.restoreBackup(id),
    onSuccess: () => {
      toast.success("Backup restored — server stopped");
      qc.invalidateQueries({ queryKey: ["server", server.id] });
      qc.invalidateQueries({ queryKey: ["servers"] });
      setRestoreId(null);
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Restore failed"),
  });

  const del = useMutation({
    mutationFn: (id: string) => api.deleteBackup(id),
    onSuccess: () => {
      toast.success("Backup deleted");
      qc.invalidateQueries({ queryKey: ["backups", server.id] });
      setDeleteId(null);
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Delete failed"),
  });

  const isServerRunning = server.state === "running" || server.state === "starting";

  return (
    <div className="space-y-6">
      {/* Action panel (Manual backup & warning notice) */}
      <div className="grid gap-4 md:grid-cols-2">
        <div className="panel flex flex-col justify-between p-5 relative overflow-hidden bg-surface/30">
          <div className="space-y-2">
            <span className="eyebrow text-gold">Manual Snapshot</span>
            <h3 className="font-display font-bold text-ink">Backup Now</h3>
            <p className="text-sm text-muted">
              Trigger a manual cold backup of the server's world and player files.
            </p>
            {isServerRunning && (
              <div className="rounded-lg border border-warn/30 bg-warn/5 p-3 text-xs text-warn">
                <strong>Notice:</strong> This will temporarily stop the server to ensure a clean backup, and restart it when complete.
              </div>
            )}
          </div>
          <div className="mt-4">
            <Button
              variant="primary"
              onClick={() => create.mutate()}
              disabled={create.isPending}
            >
              {create.isPending ? <Spinner className="h-4 w-4" /> : <Database className="h-4 w-4" />}
              {create.isPending ? "Backing up..." : "Backup Now"}
            </Button>
          </div>
        </div>

        {/* Scheduled backup configuration */}
        <div className="panel p-5 space-y-4 bg-surface/30">
          <div className="flex items-center justify-between border-b border-border/40 pb-2">
            <div className="flex items-center gap-2">
              <CalendarDays className="h-4 w-4 text-gold" />
              <span className="font-display font-bold text-ink text-sm">Scheduled Backups</span>
            </div>
            {/* Toggle switch */}
            <button
              type="button"
              role="switch"
              aria-checked={enabled}
              onClick={() => setEnabled((v) => !v)}
              className={cn(
                "relative h-6 w-11 shrink-0 rounded-full transition-colors",
                enabled ? "bg-gold" : "border border-border bg-surface-2",
              )}
            >
              <span
                className={cn(
                  "absolute top-0.5 left-0.5 h-5 w-5 rounded-full bg-bg transition-transform",
                  enabled ? "translate-x-5 bg-ink" : "translate-x-0",
                )}
              />
            </button>
          </div>

          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="interval-value" className="text-xs text-muted">Interval</Label>
              <div className="flex gap-2">
                <Input
                  id="interval-value"
                  type="number"
                  min={1}
                  value={intervalValue}
                  onChange={(e) => setIntervalValue(Number(e.target.value))}
                  disabled={!enabled}
                  className="w-20 bg-surface-2/40"
                />
                <Select
                  value={intervalUnit}
                  onValueChange={setIntervalUnit}
                  disabled={!enabled}
                >
                  <SelectTrigger className="flex-1 bg-surface-2/40">
                    <SelectValue placeholder="Unit" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="hour">Hours</SelectItem>
                    <SelectItem value="week">Weeks</SelectItem>
                    <SelectItem value="month">Months</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="space-y-1">
              <Label htmlFor="backup-keep" className="text-xs text-muted">Keep Last</Label>
              <Input
                id="backup-keep"
                type="number"
                min={1}
                max={50}
                value={keep}
                onChange={(e) => setKeep(Number(e.target.value))}
                disabled={!enabled}
                className="bg-surface-2/40"
              />
            </div>
          </div>

          <div className="pt-2">
            <Button
              variant="secondary"
              onClick={() => saveSchedule.mutate()}
              disabled={saveSchedule.isPending}
              className="w-full justify-center sm:w-auto"
            >
              {saveSchedule.isPending && <Spinner className="h-4 w-4" />}
              Save Schedule
            </Button>
          </div>
        </div>
      </div>

      {/* Backups list */}
      <div className="space-y-3">
        <h4 className="font-display font-semibold text-sm text-ink eyebrow">Available Backups</h4>
        {isLoading ? (
          <Skeleton className="h-24 w-full" />
        ) : backups && backups.length > 0 ? (
          <div className="space-y-2">
            {backups.map((b) => (
              <BackupRow
                key={b.id}
                backup={b}
                onRestore={() => setRestoreId(b.id)}
                onDelete={() => setDeleteId(b.id)}
              />
            ))}
          </div>
        ) : (
          <div className="panel grid h-32 place-items-center text-sm text-faint">
            No backup snapshots available yet.
          </div>
        )}
      </div>

      <ConfirmDialog
        open={restoreId !== null}
        onOpenChange={(o) => !o && setRestoreId(null)}
        title="Restore this backup?"
        description="The server will be stopped, its data volume wiped, and the snapshot restored. You'll start it again afterwards."
        confirmLabel="Restore"
        variant="primary"
        loading={restore.isPending}
        onConfirm={() => restoreId && restore.mutate(restoreId)}
      />

      <ConfirmDialog
        open={deleteId !== null}
        onOpenChange={(o) => !o && setDeleteId(null)}
        title="Delete this backup file?"
        description="The backup archive will be permanently removed from disk. This cannot be undone."
        confirmLabel="Delete"
        variant="danger"
        loading={del.isPending}
        onConfirm={() => deleteId && del.mutate(deleteId)}
      />
    </div>
  );
}

function BackupRow({
  backup,
  onRestore,
  onDelete,
}: {
  backup: Backup;
  onRestore: () => void;
  onDelete: () => void;
}) {
  const validated = backup.status === "validated";
  const corrupted = backup.status === "corrupted";
  const Icon = validated ? ShieldCheck : corrupted ? ShieldAlert : Clock;
  return (
    <div className="panel flex items-center justify-between gap-4 px-4 py-3">
      <div className="flex min-w-0 items-center gap-3">
        <Icon
          className={cn(
            "h-5 w-5 shrink-0",
            validated ? "text-running" : corrupted ? "text-danger" : "text-warn",
          )}
        />
        <div className="min-w-0">
          <p className="font-mono text-sm text-ink">{formatRelative(backup.created_at)}</p>
          <p className="truncate font-mono text-[0.7rem] text-faint">
            {formatBytes(backup.size_bytes)} · sha256:{shortHash(backup.sha256)}
          </p>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={onDelete}
          className="text-danger hover:border-danger/50 hover:text-danger"
          title="Delete this backup file"
        >
          <Trash2 className="h-3.5 w-3.5" />
          Delete
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={onRestore}
          disabled={!validated}
          title={validated ? "" : "Only validated backups can be restored"}
        >
          <RotateCcw className="h-3.5 w-3.5" />
          Restore
        </Button>
      </div>
    </div>
  );
}

