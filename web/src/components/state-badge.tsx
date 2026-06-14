import type { ServerState } from "@/lib/api";
import { cn } from "@/lib/utils";

const MAP: Record<ServerState, { label: string; color: string; pulse?: boolean }> = {
  running: { label: "Running", color: "text-running border-running/30 bg-running/10" },
  starting: { label: "Starting", color: "text-warn border-warn/30 bg-warn/10", pulse: true },
  stopping: { label: "Stopping", color: "text-warn border-warn/30 bg-warn/10", pulse: true },
  recovering: { label: "Recovering", color: "text-warn border-warn/30 bg-warn/10", pulse: true },
  creating: { label: "Creating", color: "text-gold border-gold/30 bg-gold/10", pulse: true },
  stopped: { label: "Stopped", color: "text-idle border-idle/30 bg-idle/10" },
  crashed: { label: "Crashed", color: "text-danger border-danger/30 bg-danger/10", pulse: true },
  corrupted: { label: "Corrupted", color: "text-danger border-danger/30 bg-danger/10" },
  unknown: { label: "Unknown", color: "text-idle border-idle/30 bg-idle/10" },
};

export function StateBadge({ state, className }: { state: ServerState; className?: string }) {
  const s = MAP[state] ?? MAP.unknown;
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
