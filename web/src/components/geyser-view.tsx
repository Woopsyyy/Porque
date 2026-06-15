import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, Download, CheckCircle, Smartphone } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError, type Server } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/misc";
import { ConfirmDialog } from "@/components/confirm-dialog";

export function GeyserView({ server }: { server: Server }) {
  const qc = useQueryClient();
  const [showConfirm, setShowConfirm] = useState(false);

  const { data: status, isLoading, isError } = useQuery({
    queryKey: ["geyser-status", server.id],
    queryFn: () => api.getGeyserStatus(server.id),
    refetchInterval: 5000,
  });

  const install = useMutation({
    mutationFn: () => api.installOrUpdateGeyser(server.id),
    onSuccess: () => {
      toast.success("Geyser & Floodgate installed/updated successfully!");
      qc.invalidateQueries({ queryKey: ["geyser-status", server.id] });
      qc.invalidateQueries({ queryKey: ["server", server.id] });
      setShowConfirm(false);
    },
    onError: (e) => {
      toast.error(e instanceof ApiError ? e.message : "Installation failed");
      setShowConfirm(false);
    },
  });

  if (isLoading) {
    return (
      <div className="flex h-44 items-center justify-center">
        <Spinner className="h-6 w-6 text-gold" />
      </div>
    );
  }

  if (isError || !status) {
    return (
      <div className="panel p-6 text-center text-sm text-danger">
        Failed to fetch Geyser status. Please make sure backend is running.
      </div>
    );
  }

  if (!status.supports_geyser) {
    return (
      <div className="panel flex items-start gap-4 border border-border bg-surface/50 p-6 max-w-xl">
        <div className="mt-0.5 rounded-lg bg-warn/10 p-2 text-warn">
          <AlertCircle className="h-5 w-5" />
        </div>
        <div>
          <h4 className="font-display font-bold text-ink">Unsupported Server Type</h4>
          <p className="mt-1.5 text-xs text-muted leading-relaxed">
            Geyser & Floodgate plugins/mods are only supported on <span className="font-medium text-ink">Paper</span> or <span className="font-medium text-ink">Fabric</span> servers.
            Your server is currently running <span className="font-medium text-gold">{status.server_type}</span>.
          </p>
          <p className="mt-3 text-[0.68rem] text-faint">
            To enable cross-play, please recreate or switch this server to Paper or Fabric.
          </p>
        </div>
      </div>
    );
  }

  const needsGeyserUpdate = status.geyser_installed && status.geyser_build < status.latest_geyser_build;
  const needsFloodgateUpdate = status.floodgate_installed && status.floodgate_build < status.latest_floodgate_build;
  const anyUpdateAvailable = needsGeyserUpdate || needsFloodgateUpdate;

  const isServerRunning = server.state === "running" || server.state === "starting";

  return (
    <div className="max-w-xl space-y-6">
      {/* Overview Card */}
      <div className="panel border border-border bg-surface/50 p-6 flex items-start gap-4">
        <div className="rounded-lg bg-gold/10 p-3 text-gold shrink-0">
          <Smartphone className="h-6 w-6" />
        </div>
        <div>
          <h3 className="font-display text-base font-bold text-ink">Geyser & Floodgate Cross-Play</h3>
          <p className="mt-1.5 text-xs text-muted leading-relaxed">
            Geyser allows Bedrock Edition players (iOS, Android, Xbox, PlayStation, Switch, Windows 10/11) to join this Java server.
            Floodgate enables them to authenticate seamlessly without needing a Java Edition account.
          </p>
        </div>
      </div>

      {/* Component Status Details */}
      <div className="grid gap-4 sm:grid-cols-2">
        {/* Geyser Status */}
        <div className="panel border border-border p-5 space-y-3">
          <span className="eyebrow">Geyser (Proxy)</span>
          <div className="flex items-center gap-2">
            {status.geyser_installed ? (
              <>
                <CheckCircle className="h-4.5 w-4.5 text-success" />
                <span className="text-xs font-semibold text-ink">Installed</span>
              </>
            ) : (
              <>
                <AlertCircle className="h-4.5 w-4.5 text-faint" />
                <span className="text-xs font-semibold text-muted">Not Installed</span>
              </>
            )}
          </div>
          <div className="space-y-1 font-mono text-[0.7rem] text-faint">
            <p>Installed: {status.geyser_installed ? `Build ${status.geyser_build}` : "None"}</p>
            <p>Latest: {status.latest_geyser_build ? `Build ${status.latest_geyser_build}` : "Checking..."}</p>
          </div>
        </div>

        {/* Floodgate Status */}
        <div className="panel border border-border p-5 space-y-3">
          <span className="eyebrow">Floodgate (Auth Bypass)</span>
          <div className="flex items-center gap-2">
            {status.floodgate_installed ? (
              <>
                <CheckCircle className="h-4.5 w-4.5 text-success" />
                <span className="text-xs font-semibold text-ink">Installed</span>
              </>
            ) : (
              <>
                <AlertCircle className="h-4.5 w-4.5 text-faint" />
                <span className="text-xs font-semibold text-muted">Not Installed</span>
              </>
            )}
          </div>
          <div className="space-y-1 font-mono text-[0.7rem] text-faint">
            <p>Installed: {status.floodgate_installed ? `Build ${status.floodgate_build}` : "None"}</p>
            <p>Latest: {status.latest_floodgate_build ? `Build ${status.latest_floodgate_build}` : "Checking..."}</p>
          </div>
        </div>
      </div>

      {/* Action Zone */}
      <div className="panel p-6 border border-border bg-surface/30 space-y-4">
        <div>
          <h4 className="font-display font-bold text-ink">Setup & Updates</h4>
          <p className="mt-1 text-xs text-muted">
            Downloads Spigot plugins or Fabric mods depending on server type, and binds them to your Bedrock tunnel local port.
          </p>
        </div>

        {anyUpdateAvailable && (
          <div className="rounded-md bg-gold/10 border border-gold/20 px-3.5 py-2.5 text-xs text-gold-bright">
            <p className="font-semibold">Update Available</p>
            <p className="mt-0.5 text-[0.7rem] text-muted">A newer version of Geyser and/or Floodgate is available on GeyserMC's API.</p>
          </div>
        )}

        <div className="flex items-center gap-4">
          <Button
            variant={anyUpdateAvailable || !status.geyser_installed ? "primary" : "outline"}
            onClick={() => {
              if (isServerRunning) {
                setShowConfirm(true);
              } else {
                install.mutate();
              }
            }}
            disabled={install.isPending}
            className="flex items-center gap-2 text-xs"
          >
            {install.isPending ? (
              <>
                <Spinner className="h-4 w-4" />
                Installing...
              </>
            ) : (
              <>
                <Download className="h-4 w-4" />
                {!status.geyser_installed ? "Install Geyser & Floodgate" : "Check & Update Version"}
              </>
            )}
          </Button>

          {isServerRunning && (
            <p className="text-[0.7rem] text-warn max-w-xs leading-normal">
              Note: The server will be stopped automatically before installing or updating files.
            </p>
          )}
        </div>
      </div>

      {/* Confirmation Dialog if running */}
      <ConfirmDialog
        open={showConfirm}
        onOpenChange={setShowConfirm}
        title="Stop Server to Install?"
        description="Installing or updating Geyser & Floodgate requires stopping the server to prevent file lock errors. Once completed, you can manually start the server again."
        confirmLabel="Stop & Install"
        variant="primary"
        loading={install.isPending}
        onConfirm={() => install.mutate()}
      />
    </div>
  );
}
