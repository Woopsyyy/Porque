import { Link } from "react-router-dom";
import { ArrowRight, Cpu, MemoryStick } from "lucide-react";
import type { Server } from "@/lib/api";
import { StateBadge } from "@/components/state-badge";

export function ServerCard({ server }: { server: Server }) {
  return (
    <Link
      to={`/servers/${server.id}`}
      className="panel group relative block overflow-hidden p-5 transition-all duration-200 hover:-translate-y-0.5 hover:border-gold/40 hover:shadow-glow"
    >
      {/* corner sheen on hover */}
      <div
        aria-hidden
        className="pointer-events-none absolute -right-16 -top-16 h-32 w-32 rounded-full bg-gold/10 opacity-0 blur-2xl transition-opacity duration-300 group-hover:opacity-100"
      />
      <div className="flex items-start justify-between">
        <span className="eyebrow">
          {server.server_type} · {server.version}
        </span>
        <StateBadge state={server.state} />
      </div>

      <h3 className="mt-3 font-display text-xl font-bold tracking-tight text-ink">{server.name}</h3>

      <div className="mt-4 flex items-center gap-4 font-mono text-xs text-muted">
        <span className="inline-flex items-center gap-1.5">
          <Cpu className="h-3.5 w-3.5 text-faint" />
          {server.cpu_cores} cores
        </span>
        <span className="inline-flex items-center gap-1.5">
          <MemoryStick className="h-3.5 w-3.5 text-faint" />
          {server.memory_mb} MB
        </span>
      </div>

      <div className="mt-4 flex items-center justify-end text-sm font-medium text-muted transition-colors group-hover:text-gold">
        Open
        <ArrowRight className="ml-1 h-4 w-4 transition-transform group-hover:translate-x-0.5" />
      </div>
    </Link>
  );
}
