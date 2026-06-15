import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Cable, Plus, LogOut, KeyRound, Copy, RefreshCw, Smartphone, Monitor } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import { useWebSocket } from "@/lib/ws";
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

  const { data: servers, isLoading: serversLoading } = useQuery({
    queryKey: ["servers"],
    queryFn: api.listServers,
  });

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

  // Create both Java and Bedrock tunnels at once
  const createBothTunnels = useMutation({
    mutationFn: (serverId: string) => api.createJavaAndBedrockTunnels(serverId),
    onSuccess: () => {
      toast.success("Java & Bedrock tunnels created!");
      qc.invalidateQueries({ queryKey: ["tunnels"] });
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Failed to create tunnels"),
  });

  // Rescan tunnels
  const rescan = useMutation({
    mutationFn: (serverId: string) => api.rescanTunnel(serverId),
    onSuccess: (t) => {
      if (t.some((x) => x.public_address)) {
        toast.success("Tunnels address resolved!");
      } else {
        toast.message("Checking Playit.gg — address still being assigned");
      }
      qc.invalidateQueries({ queryKey: ["tunnels"] });
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Rescan failed"),
  });

  // Detach tunnels
  const detach = useMutation({
    mutationFn: (args: { serverId: string; proto: "tcp" | "udp" }) =>
      api.detachTunnel(args.serverId, args.proto),
    onSuccess: () => {
      toast.success("Tunnel detached");
      qc.invalidateQueries({ queryKey: ["tunnels"] });
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Detach failed"),
  });

  const activeAccount = accounts?.find((a) => a.status !== "claiming");
  const claimingAccount = accounts?.find((a) => a.status === "claiming");

  // Find the running server
  const runningServer = servers?.find((s) => s.state === "running" || s.state === "starting");

  // WebSocket realtime update subscription
  useWebSocket(runningServer ? `/ws/playit/${runningServer.id}` : null, () => {
    qc.invalidateQueries({ queryKey: ["tunnels"] });
  });

  // Filter tunnels for the running server
  const javaTunnel = tunnels?.find((t) => t.proto === "tcp" && runningServer && t.server_id === runningServer.id);
  const bedrockTunnel = tunnels?.find((t) => t.proto === "udp" && runningServer && t.server_id === runningServer.id);

  const hasTunnels = !!(javaTunnel || bedrockTunnel);

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

      {/* Tunnels Section */}
      {activeAccount && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="font-display text-base font-bold text-ink">Active Tunnels</h3>
              {runningServer && (
                <p className="text-xs text-muted">
                  Exposing server: <span className="text-gold font-semibold">{runningServer.name}</span>
                </p>
              )}
            </div>

            {runningServer && (
              <div className="flex items-center gap-2">
                {hasTunnels && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => rescan.mutate(runningServer.id)}
                    disabled={rescan.isPending}
                    className="text-xs flex items-center gap-1.5"
                  >
                    {rescan.isPending ? <Spinner className="h-3.5 w-3.5" /> : <RefreshCw className="h-3.5 w-3.5" />}
                    Rescan
                  </Button>
                )}
                {!hasTunnels && (
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={() => createBothTunnels.mutate(runningServer.id)}
                    disabled={createBothTunnels.isPending}
                    className="text-xs flex items-center gap-1.5"
                  >
                    {createBothTunnels.isPending ? <Spinner className="h-3.5 w-3.5" /> : <Plus className="h-3.5 w-3.5" />}
                    Add Tunnels
                  </Button>
                )}
              </div>
            )}
          </div>

          {tunnelsLoading || serversLoading ? (
            <Skeleton className="h-28 w-full" />
          ) : !runningServer ? (
            <div className="panel flex flex-col items-center justify-center p-8 text-center border border-border bg-surface/30">
              <Cable className="h-7 w-7 text-faint mb-2.5" />
              <p className="text-sm text-muted font-medium">No Server is Currently Running</p>
              <p className="text-xs text-faint mt-1 max-w-sm">
                Start a server from the servers list page to add or manage public playit.gg tunnels.
              </p>
            </div>
          ) : !hasTunnels ? (
            <div className="panel flex flex-col items-center justify-center p-8 text-center border border-border bg-surface/30">
              <Cable className="h-7 w-7 text-faint mb-2.5" />
              <p className="text-sm text-muted font-medium">No Tunnels Created Yet</p>
              <p className="text-xs text-faint mt-1 max-w-sm">
                Expose this server to the internet by clicking "Add Tunnels" above. It will automatically create both Java and Bedrock connection paths.
              </p>
            </div>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2">
              {/* Java Tunnel Card */}
              <div className="panel border border-border p-5 space-y-4">
                <div className="flex items-center justify-between">
                  <span className="inline-flex items-center gap-2 font-display text-sm font-semibold text-ink">
                    <Monitor className="h-4.5 w-4.5 text-gold shrink-0" />
                    Java Connection (TCP)
                  </span>
                  {javaTunnel ? (
                    <TunnelStatusBadge status={javaTunnel.status} />
                  ) : (
                    <span className="text-[0.68rem] text-faint font-semibold uppercase tracking-wider bg-surface-dark border border-border/60 px-2 py-0.5 rounded">
                      Not Configured
                    </span>
                  )}
                </div>

                {javaTunnel && (
                  <div className="space-y-3">
                    <div>
                      <p className="eyebrow mb-1">Public address</p>
                      {javaTunnel.public_address ? (
                        <button
                          onClick={() => {
                            navigator.clipboard?.writeText(javaTunnel.public_address!);
                            toast.success("Java address copied");
                          }}
                          className="group inline-flex items-center gap-2 rounded-md border border-border bg-bg/50 px-2.5 py-1.5 font-mono text-[0.72rem] text-gold w-full text-left justify-between"
                        >
                          <span className="truncate">{javaTunnel.public_address}</span>
                          <Copy className="h-3 w-3 text-faint shrink-0 group-hover:text-gold" />
                        </button>
                      ) : (
                        <p className="font-mono text-xs text-faint">Awaiting address allocation...</p>
                      )}
                    </div>
                    <Button
                      variant="danger"
                      size="sm"
                      onClick={() => detach.mutate({ serverId: runningServer.id, proto: "tcp" })}
                      disabled={detach.isPending}
                      className="text-xs"
                    >
                      Delete Java Tunnel
                    </Button>
                  </div>
                )}
              </div>

              {/* Bedrock Tunnel Card */}
              <div className="panel border border-border p-5 space-y-4">
                <div className="flex items-center justify-between">
                  <span className="inline-flex items-center gap-2 font-display text-sm font-semibold text-ink">
                    <Smartphone className="h-4.5 w-4.5 text-gold shrink-0" />
                    Bedrock Connection (UDP)
                  </span>
                  {bedrockTunnel ? (
                    <TunnelStatusBadge status={bedrockTunnel.status} />
                  ) : (
                    <span className="text-[0.68rem] text-faint font-semibold uppercase tracking-wider bg-surface-dark border border-border/60 px-2 py-0.5 rounded">
                      Not Configured
                    </span>
                  )}
                </div>

                {bedrockTunnel && (
                  <div className="space-y-3">
                    <div>
                      <p className="eyebrow mb-1">Public address</p>
                      {bedrockTunnel.public_address ? (
                        <button
                          onClick={() => {
                            navigator.clipboard?.writeText(bedrockTunnel.public_address!);
                            toast.success("Bedrock address copied");
                          }}
                          className="group inline-flex items-center gap-2 rounded-md border border-border bg-bg/50 px-2.5 py-1.5 font-mono text-[0.72rem] text-gold w-full text-left justify-between"
                        >
                          <span className="truncate">{bedrockTunnel.public_address}</span>
                          <Copy className="h-3 w-3 text-faint shrink-0 group-hover:text-gold" />
                        </button>
                      ) : (
                        <p className="font-mono text-xs text-faint">Awaiting address allocation...</p>
                      )}
                    </div>
                    <Button
                      variant="danger"
                      size="sm"
                      onClick={() => detach.mutate({ serverId: runningServer.id, proto: "udp" })}
                      disabled={detach.isPending}
                      className="text-xs"
                    >
                      Delete Bedrock Tunnel
                    </Button>
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      )}

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
