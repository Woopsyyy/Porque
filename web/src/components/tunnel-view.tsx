import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Cable, Copy, ExternalLink, Link2Off, Plus, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError, type PlayitAccount, type Server, type ServerTunnel } from "@/lib/api";
import { useWebSocket } from "@/lib/ws";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/misc";
import { TunnelStatusBadge } from "@/components/tunnel-status-badge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

type AccountWithClaim = PlayitAccount & { claim_url?: string };

const protoLabel = (proto: string) => (proto === "udp" ? "Bedrock" : "Java");

export function TunnelView({ server }: { server: Server }) {
  const qc = useQueryClient();
  const running = server.state === "running";

  const { data: tunnels } = useQuery({
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
      if (t.some((x) => x.public_address)) toast.success("Public address assigned");
      else toast.message("Still being assigned — try again in a moment");
      invalidate();
    },
    onError: onErr,
  });
  const detach = useMutation({
    mutationFn: (proto: "tcp" | "udp") => api.detachTunnel(server.id, proto),
    onSuccess: () => {
      toast.success("Tunnel detached");
      invalidate();
    },
    onError: onErr,
  });

  const list = tunnels ?? [];
  const hasJava = list.some((t) => t.proto === "tcp");
  const hasBedrock = list.some((t) => t.proto === "udp");
  const claiming = (accounts as AccountWithClaim[] | undefined)?.find((a) => a.status === "claiming");

  return (
    <div className="max-w-xl space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2 text-muted">
          <Cable className="h-4 w-4 text-gold" />
          <span className="eyebrow !text-muted">Playit.gg tunnels</span>
        </div>
        <div className="flex items-center gap-2">
          {list.length > 0 && (
            <Button variant="outline" size="sm" onClick={() => rescan.mutate()} disabled={rescan.isPending}>
              {rescan.isPending ? <Spinner className="h-3.5 w-3.5" /> : <RefreshCw className="h-3.5 w-3.5" />}
              Rescan
            </Button>
          )}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="primary" size="sm" disabled={!running || create.isPending}>
                {create.isPending ? <Spinner className="h-3.5 w-3.5" /> : <Plus className="h-3.5 w-3.5" />}
                Add tunnel
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                disabled={hasJava}
                onSelect={() => create.mutate("java")}
              >
                <Cable className="h-4 w-4" />
                Create Java tunnel
              </DropdownMenuItem>
              <DropdownMenuItem
                disabled={hasBedrock}
                onSelect={() => create.mutate("bedrock")}
              >
                <Cable className="h-4 w-4" />
                Create Bedrock tunnel
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {claiming && (
        <div className="panel p-5">
          <span className="eyebrow">Linking Playit.gg</span>
          <p className="mt-2 text-sm text-muted">
            One-time setup: approve the bundled agent in your browser, then tunnels are automatic.
          </p>
          {claiming.claim_url && (
            <Button asChild variant="primary" size="sm" className="mt-3">
              <a href={claiming.claim_url} target="_blank" rel="noreferrer">
                <ExternalLink className="h-4 w-4" />
                Approve agent
              </a>
            </Button>
          )}
        </div>
      )}

      {list.length === 0 ? (
        <div className="panel px-6 py-10 text-center">
          <p className="text-sm text-muted">
            No tunnels yet. Use <span className="text-ink">Add tunnel</span> to expose this server —
            Java (TCP) and/or Bedrock (UDP) can run at the same time.
          </p>
          {!running && (
            <p className="mt-2 text-xs text-warn">The server must be running to create a tunnel.</p>
          )}
        </div>
      ) : (
        <div className="space-y-2">
          {list.map((t) => (
            <TunnelRow
              key={t.id}
              tunnel={t}
              onDetach={() => detach.mutate(t.proto === "udp" ? "udp" : "tcp")}
              detaching={detach.isPending && detach.variables === (t.proto === "udp" ? "udp" : "tcp")}
            />
          ))}
        </div>
      )}

      <p className="font-mono text-[0.68rem] text-faint">
        Java → TCP · 25565 &nbsp;·&nbsp; Bedrock → UDP · 19132 (needs Geyser)
      </p>
    </div>
  );
}

function TunnelRow({
  tunnel,
  onDetach,
  detaching,
}: {
  tunnel: ServerTunnel;
  onDetach: () => void;
  detaching: boolean;
}) {
  return (
    <div className="panel p-5">
      <div className="flex items-center justify-between">
        <span className="inline-flex items-center gap-2 font-display text-base font-semibold text-ink">
          <Cable className="h-4 w-4 text-gold" />
          {protoLabel(tunnel.proto)}
        </span>
        <TunnelStatusBadge status={tunnel.status} />
      </div>

      <div className="mt-4">
        <p className="eyebrow mb-1.5">Public address</p>
        {tunnel.public_address ? (
          <button
            onClick={() => {
              navigator.clipboard?.writeText(tunnel.public_address!);
              toast.success("Address copied");
            }}
            className="group inline-flex items-center gap-2 rounded-md border border-border bg-bg/60 px-3 py-2 font-mono text-sm text-gold"
          >
            {tunnel.public_address}
            <Copy className="h-3.5 w-3.5 text-faint group-hover:text-gold" />
          </button>
        ) : (
          <p className="font-mono text-sm text-faint">
            Not assigned yet — press Rescan once the agent connects.
          </p>
        )}
      </div>

      <div className="mt-4 border-t border-border pt-3">
        <Button variant="danger" size="sm" onClick={onDetach} disabled={detaching}>
          {detaching ? <Spinner className="h-3.5 w-3.5" /> : <Link2Off className="h-3.5 w-3.5" />}
          Detach
        </Button>
      </div>
    </div>
  );
}
