import type { TunnelStatus } from "@/lib/api";
import { cn } from "@/lib/utils";

const MAP: Record<TunnelStatus, { label: string; color: string; pulse?: boolean }> = {
  connected: { label: "Connected", color: "text-running border-running/30 bg-running/10" },
  starting: { label: "Starting", color: "text-warn border-warn/30 bg-warn/10", pulse: true },
  disconnected: { label: "Disconnected", color: "text-idle border-idle/30 bg-idle/10" },
  error: { label: "Error", color: "text-danger border-danger/30 bg-danger/10" },
};

export function TunnelStatusBadge({ status, className }: { status: TunnelStatus; className?: string }) {
  const s = MAP[status] ?? MAP.disconnected;
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 font-mono text-[0.7rem] uppercase tracking-wider",
        s.color,
        className,
      )}
    >
      <span className={cn("h-1.5 w-1.5 rounded-full bg-current", s.pulse && "animate-pulsedot")} />
      {s.label}
    </span>
  );
}
