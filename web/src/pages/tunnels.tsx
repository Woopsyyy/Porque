import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { Cable, ArrowUpRight, Plus, LogOut, KeyRound } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { TunnelStatusBadge } from "@/components/tunnel-status-badge";
import { Skeleton, Spinner } from "@/components/ui/misc";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Card, CardContent } from "@/components/ui/card";

export default function TunnelsPage() {
  const qc = useQueryClient();
  const [showDisconnectConfirm, setShowDisconnectConfirm] = useState(false);

  // Poll accounts rapidly if one is currently in the claiming process
  const { data: accounts, isLoading: accountsLoading } = useQuery({
    queryKey: ["accounts"],
    queryFn: api.listAccounts,
    refetchInterval: (query) => {
      const list = query.state.data;
      if (list && list.some((a) => a.status === "claiming")) {
        return 3000;
      }
      return false;
    },
  });

  const { data: tunnels, isLoading: tunnelsLoading } = useQuery({
    queryKey: ["tunnels"],
    queryFn: api.listTunnels,
    refetchInterval: 6000,
  });

  const { data: servers } = useQuery({ queryKey: ["servers"], queryFn: api.listServers });

  const claim = useMutation({
    mutationFn: () => api.startPlayitClaim(),
    onSuccess: (data) => {
      if (data.claim_url) {
        window.open(data.claim_url, "_blank");
        toast.success("Opening Playit.gg claim link...");
      }
      qc.invalidateQueries({ queryKey: ["accounts"] });
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Connect failed"),
  });

  const disconnect = useMutation({
    mutationFn: (id: string) => api.deleteAccount(id),
    onSuccess: () => {
      toast.success("Account disconnected");
      qc.invalidateQueries({ queryKey: ["accounts"] });
      qc.invalidateQueries({ queryKey: ["tunnels"] });
      setShowDisconnectConfirm(false);
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Disconnect failed"),
  });

  const nameFor = (id: string) => servers?.find((s) => s.id === id)?.name ?? id.slice(0, 8);

  const activeAccount = accounts?.find((a) => a.status !== "claiming");
  const claimingAccount = accounts?.find((a) => a.status === "claiming");

  return (
    <div className="space-y-7">
      <PageHeader
        title="Tunnels"
        subtitle="Manage Playit.gg connections exposing your Minecraft servers."
      />

      {/* Account Status / Claim Flow Card */}
      {accountsLoading ? (
        <Skeleton className="h-24 w-full" />
      ) : activeAccount ? (
        <Card className="panel border border-border bg-surface/50 backdrop-blur-md px-5 py-4">
          <CardContent className="flex items-center justify-between gap-4 p-0">
            <div className="flex items-center gap-3">
              <span className="grid h-9 w-9 place-items-center rounded-md bg-gold/12 text-gold">
                <KeyRound className="h-4 w-4" />
              </span>
              <div>
                <p className="font-display text-sm font-bold text-ink">Playit Account Connected</p>
                <p className="text-xs text-muted">
                  Name: <span className="text-gold font-mono">{activeAccount.name}</span>
                </p>
              </div>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowDisconnectConfirm(true)}
              className="flex items-center gap-1.5 text-xs text-muted hover:text-danger"
            >
              <LogOut className="h-3.5 w-3.5" />
              Disconnect Account
            </Button>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-4">
          {claimingAccount && claimingAccount.claim_url && (
            <div className="rounded-md bg-gold/10 border border-gold/20 p-4 text-sm text-gold-bright flex flex-col sm:flex-row items-center justify-between gap-3 animate-pulse">
              <div>
                <p className="font-bold">Action Required</p>
                <p className="text-xs text-muted">Please approve this agent setup in your browser window.</p>
              </div>
              <Button asChild variant="primary" size="sm">
                <a href={claimingAccount.claim_url} target="_blank" rel="noopener noreferrer">
                  Link Playit.gg Account
                </a>
              </Button>
            </div>
          )}

          <Card className="panel border border-border bg-surface/50 backdrop-blur-md px-5 py-6">
            <CardContent className="flex flex-col items-center gap-4 text-center p-0">
              <KeyRound className="h-8 w-8 text-faint" />
              <div>
                <h3 className="font-display text-base font-bold text-ink">No Playit Account Connected</h3>
                <p className="mt-1 max-w-sm text-xs text-muted">
                  Connect a Playit.gg account to start exposing Minecraft servers to the internet automatically.
                </p>
              </div>
              <Button
                variant="primary"
                onClick={() => claim.mutate()}
                disabled={claim.isPending || !!claimingAccount}
                className="flex items-center gap-2 text-xs"
              >
                {claim.isPending && <Spinner className="h-4 w-4" />}
                <Plus className="h-4 w-4" />
                Connect Account
              </Button>
            </CardContent>
          </Card>
        </div>
      )}

      {/* Tunnels List Section */}
      <div className="space-y-4">
        <h3 className="font-display text-base font-bold text-ink">Active Tunnels</h3>
        {tunnelsLoading ? (
          <Skeleton className="h-24 w-full" />
        ) : tunnels && tunnels.length > 0 ? (
          <div className="space-y-2">
            {tunnels.map((t) => (
              <div key={t.id} className="panel flex items-center justify-between gap-4 px-5 py-4">
                <div className="flex min-w-0 items-center gap-3">
                  <Cable className="h-5 w-5 shrink-0 text-gold" />
                  <div className="min-w-0">
                    <Link
                      to={`/servers/${t.server_id}`}
                      className="group inline-flex items-center gap-1 font-display text-base font-semibold text-ink hover:text-gold"
                    >
                      {nameFor(t.server_id)}
                      <ArrowUpRight className="h-3.5 w-3.5 opacity-0 transition-opacity group-hover:opacity-100" />
                    </Link>
                    <p className="truncate font-mono text-xs text-faint">
                      {t.public_address ?? "awaiting address…"}
                    </p>
                  </div>
                </div>
                <TunnelStatusBadge status={t.status} />
              </div>
            ))}
          </div>
        ) : (
          <div className="panel grid h-44 place-items-center text-center text-sm text-faint">
            No active tunnels. Attach one from a server&apos;s Tunnel tab.
          </div>
        )}
      </div>

      {/* Disconnect Confirmation */}
      <ConfirmDialog
        open={showDisconnectConfirm}
        onOpenChange={(o) => !o && setShowDisconnectConfirm(false)}
        title="Disconnect Playit.gg Account?"
        description="Disconnecting this account will remove it from the dashboard, detach all active tunnels, and stop all associated agent sidecar containers."
        confirmLabel="Disconnect"
        variant="danger"
        loading={disconnect.isPending}
        onConfirm={() => {
          const id = activeAccount?.id;
          if (id) {
            disconnect.mutate(id);
          }
        }}
      />
    </div>
  );
}
