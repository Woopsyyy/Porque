import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CalendarClock,
  Clock,
  DatabaseBackup,
  RotateCcw,
  ShieldAlert,
  ShieldCheck,
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";

const INTERVALS = [
  { label: "Hourly", minutes: 60 },
  { label: "Every 6 hours", minutes: 360 },
  { label: "Every 12 hours", minutes: 720 },
  { label: "Daily", minutes: 1440 },
];
const intervalLabel = (m: number) =>
  INTERVALS.find((i) => i.minutes === m)?.label ?? `Every ${m} min`;

export function BackupsView({ server }: { server: Server }) {
  const qc = useQueryClient();
  const running = server.state === "running";
  const [restoreId, setRestoreId] = useState<string | null>(null);

  const { data: backups, isLoading } = useQuery({
    queryKey: ["backups", server.id],
    queryFn: () => api.listBackups(server.id),
  });

  const create = useMutation({
    mutationFn: () => api.createBackup(server.id),
    onSuccess: () => {
      toast.success("Backup created and verified");
      qc.invalidateQueries({ queryKey: ["backups", server.id] });
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Backup failed"),
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

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="max-w-md text-sm text-muted">
          Zero-downtime snapshots of the world volume — frozen with RCON, compressed, and
          SHA-256 verified.
        </p>
        <div className="flex items-center gap-2">
          <ScheduleButton server={server} />
          <Button
            variant="primary"
            onClick={() => create.mutate()}
            disabled={!running || create.isPending}
            title={running ? "" : "Server must be running for a zero-downtime backup"}
          >
            {create.isPending ? <Spinner className="h-4 w-4" /> : <DatabaseBackup className="h-4 w-4" />}
            Create backup
          </Button>
        </div>
      </div>

      {server.backup_enabled && (
        <div className="flex items-center gap-2 rounded-md border border-running/25 bg-running/5 px-4 py-2.5 text-xs text-running">
          <CalendarClock className="h-3.5 w-3.5" />
          Auto-backup {intervalLabel(server.backup_interval_minutes).toLowerCase()} · keep last{" "}
          {server.backup_keep}
          {server.backup_last_run && (
            <span className="text-faint"> · last {formatRelative(server.backup_last_run)}</span>
          )}
        </div>
      )}

      {!running && (
        <div className="rounded-md border border-warn/25 bg-warn/5 px-4 py-2.5 text-xs text-warn">
          The server must be running to capture a zero-downtime backup.
        </div>
      )}

      {isLoading ? (
        <Skeleton className="h-24 w-full" />
      ) : backups && backups.length > 0 ? (
        <div className="space-y-2">
          {backups.map((b) => (
            <BackupRow key={b.id} backup={b} onRestore={() => setRestoreId(b.id)} />
          ))}
        </div>
      ) : (
        <div className="panel grid h-40 place-items-center text-sm text-faint">
          No backups yet.
        </div>
      )}

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
    </div>
  );
}

function BackupRow({ backup, onRestore }: { backup: Backup; onRestore: () => void }) {
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
  );
}

function ScheduleButton({ server }: { server: Server }) {
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const [enabled, setEnabled] = useState(server.backup_enabled);
  const [interval, setIntervalMin] = useState(String(server.backup_interval_minutes || 360));
  const [keep, setKeep] = useState(String(server.backup_keep || 5));

  // Reset fields to the server's current schedule each time the modal opens.
  const onOpenChange = (o: boolean) => {
    if (o) {
      setEnabled(server.backup_enabled);
      setIntervalMin(String(server.backup_interval_minutes || 360));
      setKeep(String(server.backup_keep || 5));
    }
    setOpen(o);
  };

  const save = useMutation({
    mutationFn: () =>
      api.updateBackupSchedule(server.id, {
        enabled,
        interval_minutes: Number(interval),
        keep: Math.max(1, Number(keep) || 5),
      }),
    onSuccess: () => {
      toast.success(enabled ? "Backup schedule enabled" : "Backup schedule disabled");
      qc.invalidateQueries({ queryKey: ["server", server.id] });
      qc.invalidateQueries({ queryKey: ["servers"] });
      setOpen(false);
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Could not save schedule"),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <Button variant="secondary" onClick={() => onOpenChange(true)}>
        <CalendarClock className="h-4 w-4" />
        Schedule
      </Button>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Scheduled backups</DialogTitle>
          <DialogDescription>
            Automatically capture zero-downtime backups (world + all player data). The oldest is
            deleted once the limit is reached.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-5">
          <div className="flex items-center justify-between">
            <Label>Enable schedule</Label>
            <button
              type="button"
              role="switch"
              aria-checked={enabled}
              onClick={() => setEnabled((e) => !e)}
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

          <div className="space-y-1.5">
            <Label>Interval</Label>
            <Select value={interval} onValueChange={setIntervalMin} disabled={!enabled}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {INTERVALS.map((i) => (
                  <SelectItem key={i.minutes} value={String(i.minutes)}>
                    {i.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="keep">Keep last</Label>
            <Input
              id="keep"
              type="number"
              min={1}
              max={50}
              value={keep}
              onChange={(e) => setKeep(e.target.value)}
              disabled={!enabled}
            />
            <p className="font-mono text-[0.68rem] text-faint">
              older backups are removed once this many are kept.
            </p>
          </div>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button variant="primary" onClick={() => save.mutate()} disabled={save.isPending}>
            {save.isPending && <Spinner className="h-4 w-4" />}
            Save schedule
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
