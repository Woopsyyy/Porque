import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
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
import { Skeleton, Spinner } from "@/components/ui/misc";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { cn } from "@/lib/utils";

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
