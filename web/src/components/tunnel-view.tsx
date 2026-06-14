import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Cable, Copy, ExternalLink, Link2Off, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError, type PlayitAccount, type Server } from "@/lib/api";
import { useWebSocket } from "@/lib/ws";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/misc";
import { TunnelStatusBadge } from "@/components/tunnel-status-badge";

// The bundled-account mock surfaces a claim_url while linking.
type AccountWithClaim = PlayitAccount & { claim_url?: string };

export function TunnelView({ server }: { server: Server }) {
  const qc = useQueryClient();
  const running = server.state === "running";

  const { data: tunnel } = useQuery({
    queryKey: ["tunnel", server.id],
    queryFn: () => api.getTunnels(server.id),
    refetchInterval: 5000,
  });
  const { data: accounts } = useQuery({
    queryKey: ["accounts"],
    queryFn: api.listAccounts,
    refetchInterval: 3000,
  });

  useWebSocket(`/ws/playit/${server.id}`, () => {
    qc.invalidateQueries({ queryKey: ["tunnel", server.id] });
  });

  const invalidate = () => qc.invalidateQueries({ queryKey: ["tunnel", server.id] });
  const onErr = (e: unknown) => toast.error(e instanceof ApiError ? e.message : "Action failed");

  const create = useMutation({
    mutationFn: (kind: "java" | "bedrock") => api.createTunnel(server.id, kind),
    onSuccess: () => {
      toast.success("Tunnel created");
      invalidate();
      qc.invalidateQueries({ queryKey: ["accounts"] });
    },
    onError: onErr,
  });
  const rescan = useMutation({
    mutationFn: () => api.rescanTunnel(server.id),
    onSuccess: (t) => {
      const first = Array.isArray(t) && t.length > 0 ? t[0] : null;
      if (first?.public_address) toast.success("Public address assigned");
      else toast.message("Still being assigned — try again in a moment");
      invalidate();
    },
    onError: onErr,
  });
  const detach = useMutation({
    mutationFn: () => api.detachTunnel(server.id),
    onSuccess: () => {
      toast.success("Tunnel detached");
      invalidate();
    },
    onError: onErr,
  });

  const active = Array.isArray(tunnel) && tunnel.length > 0 ? tunnel[0] : null;
  const claiming = (accounts as AccountWithClaim[] | undefined)?.find(
    (a) => a.status === "claiming",
  );

  // ---- Active tunnel -------------------------------------------------------
  if (active) {
    return (
      <div className="panel max-w-xl p-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2 text-muted">
            <Cable className="h-4 w-4 text-gold" />
            <span className="eyebrow !text-muted">Playit tunnel</span>
          </div>
          <TunnelStatusBadge status={active.status} />
        </div>

        <div className="mt-5">
          <p className="eyebrow mb-1.5">Public address</p>
          {active.public_address ? (
            <button
              onClick={() => {
                navigator.clipboard?.writeText(active.public_address!);
                toast.success("Address copied");
              }}
              className="group inline-flex items-center gap-2 rounded-md border border-border bg-bg/60 px-3 py-2 font-mono text-sm text-gold"
            >
              {active.public_address}
              <Copy className="h-3.5 w-3.5 text-faint group-hover:text-gold" />
            </button>
          ) : (
            <p className="font-mono text-sm text-faint">
              Not assigned yet — press Rescan once the agent connects.
            </p>
          )}
        </div>

        <div className="mt-6 flex items-center gap-2 border-t border-border pt-4">
          <Button variant="outline" onClick={() => rescan.mutate()} disabled={rescan.isPending}>
            {rescan.isPending ? <Spinner className="h-4 w-4" /> : <RefreshCw className="h-4 w-4" />}
            Rescan
          </Button>
          <Button variant="danger" onClick={() => detach.mutate()} disabled={detach.isPending}>
            {detach.isPending ? <Spinner className="h-4 w-4" /> : <Link2Off className="h-4 w-4" />}
            Detach tunnel
          </Button>
        </div>
      </div>
    );
  }

  // ---- Linking the bundled account ----------------------------------------
  if (claiming) {
    return (
      <div className="panel max-w-xl p-6">
        <div className="flex items-center gap-2 text-muted">
          <Cable className="h-4 w-4 text-gold" />
          <span className="eyebrow !text-muted">Linking Playit.gg</span>
        </div>
        <p className="mt-3 text-sm text-muted">
          One-time setup: approve the bundled agent in your browser. After that, tunnels are
          fully automatic.
        </p>
        {claiming.claim_url && (
          <Button asChild variant="primary" className="mt-4">
            <a href={claiming.claim_url} target="_blank" rel="noreferrer">
              <ExternalLink className="h-4 w-4" />
              Approve agent
            </a>
          </Button>
        )}
        <p className="mt-3 inline-flex items-center gap-2 font-mono text-xs text-faint">
          <Spinner className="h-3.5 w-3.5" />
          Waiting for approval…
        </p>
      </div>
    );
  }

  // ---- Create a tunnel -----------------------------------------------------
  return (
    <div className="panel max-w-xl p-6">
      <div className="flex items-center gap-2 text-muted">
        <Cable className="h-4 w-4 text-gold" />
        <span className="eyebrow !text-muted">Expose with Playit.gg</span>
      </div>
      <p className="mt-3 text-sm text-muted">
        Launches a Playit agent that shares this server&apos;s network and forwards its port
        through a public tunnel. Choose the protocol for how players connect.
      </p>

      <div className="mt-5 flex flex-wrap gap-2">
        <Button
          variant="primary"
          onClick={() => create.mutate("java")}
          disabled={!running || create.isPending}
          title={running ? "" : "Server must be running"}
        >
          {create.isPending && create.variables === "java" ? (
            <Spinner className="h-4 w-4" />
          ) : (
            <Cable className="h-4 w-4" />
          )}
          Create Java tunnel
        </Button>
        <Button
          variant="secondary"
          onClick={() => create.mutate("bedrock")}
          disabled={!running || create.isPending}
          title={running ? "" : "Server must be running"}
        >
          {create.isPending && create.variables === "bedrock" ? (
            <Spinner className="h-4 w-4" />
          ) : (
            <Cable className="h-4 w-4" />
          )}
          Create Bedrock tunnel
        </Button>
      </div>

      <p className="mt-3 font-mono text-[0.7rem] text-faint">
        Java → TCP · port 25565 &nbsp;·&nbsp; Bedrock → UDP · port 19132 (needs Geyser)
      </p>
      {!running && (
        <p className="mt-2 text-xs text-warn">The server must be running to create a tunnel.</p>
      )}
    </div>
  );
}
