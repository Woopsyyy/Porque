import { useEffect, useMemo, useState, type ComponentType } from "react";
import { useQuery } from "@tanstack/react-query";
import { Area, AreaChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { Activity, Cpu, HardDrive, MemoryStick, Users } from "lucide-react";
import { api, type Server } from "@/lib/api";
import { useWebSocket } from "@/lib/ws";
import { formatBytes } from "@/lib/format";

interface Point {
  time: string;
  cpu: number;
  memPct: number;
  memBytes: number;
  players: number;
  max: number;
  latency: number;
  storage: number;
}

const MAX_POINTS = 120;
const fmtTime = (ts: number) =>
  new Date(ts).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });

export function MetricsView({ server }: { server: Server }) {
  const running = server.state === "running";
  const memLimit = server.memory_mb * 1024 * 1024;
  const [points, setPoints] = useState<Point[]>([]);

  const { data: history } = useQuery({
    queryKey: ["metrics", server.id],
    queryFn: () => api.getMetrics(server.id, MAX_POINTS),
  });

  useEffect(() => {
    if (!history) return;
    setPoints(
      history
        .slice()
        .reverse()
        .map((m) => ({
          time: fmtTime(new Date(m.recorded_at).getTime()),
          cpu: Math.round(m.cpu_pct),
          memPct: Math.round((m.mem_bytes / memLimit) * 100),
          memBytes: m.mem_bytes,
          players: m.player_count,
          max: m.max_players,
          latency: m.latency_ms ?? 0,
          storage: m.storage_bytes,
        })),
    );
  }, [history, memLimit]);

  useWebSocket(running ? `/ws/status/${server.id}` : null, (data) => {
    try {
      console.log("[MetricsView] Raw metrics event data received:", data);
      const m = JSON.parse(data);
      if (m.type !== "metrics") return;
      console.log("[MetricsView] Appending point:", m);
      setPoints((prev) => {
        const next = [
          ...prev,
          {
            time: fmtTime(Date.now()),
            cpu: Math.round(m.cpu_pct),
            memPct: Math.round((m.mem_bytes / memLimit) * 100),
            memBytes: m.mem_bytes,
            players: m.player_count,
            max: m.max_players,
            latency: m.latency_ms ?? 0,
            storage: m.storage_bytes ?? 0,
          },
        ].slice(-MAX_POINTS);
        console.log("[MetricsView] New points length:", next.length);
        return next;
      });
    } catch (err) {
      console.error("[MetricsView] Failed to parse/process metrics event:", err);
    }
  });

  const latest = points[points.length - 1];

  return (
    <div className="space-y-5">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard icon={Cpu} label="CPU" value={latest ? `${latest.cpu}%` : "—"} sub="of allocated cores" />
        <StatCard
          icon={MemoryStick}
          label="Memory"
          value={latest ? formatBytes(latest.memBytes) : "—"}
          sub={`of ${server.memory_mb} MB`}
        />
        <StatCard
          icon={Activity}
          label="Latency"
          value={latest && latest.latency ? `${latest.latency} ms` : "—"}
          sub="server response"
        />
        <StatCard
          icon={Users}
          label="Players"
          value={latest ? `${latest.players}/${latest.max || "—"}` : "—"}
          sub="online now"
        />
      </div>

      {points.length === 0 ? (
        <div className="panel grid h-64 place-items-center text-sm text-faint">
          {running ? "Collecting metrics…" : "No metrics yet — start the server to gather data."}
        </div>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          <ChartCard title="CPU usage" color="#E8B931" dataKey="cpu" data={points} percent fmt={(v) => `${v}%`} />
          <ChartCard title="Memory usage" color="#5BA8FF" dataKey="memPct" data={points} percent fmt={(v) => `${v}%`} />
          <ChartCard title="Latency" color="#34D399" dataKey="latency" data={points} fmt={(v) => `${v} ms`} />
          <ChartCard
            title="Storage"
            color="#C084FC"
            dataKey="storage"
            data={points}
            icon={HardDrive}
            fmt={(v) => formatBytes(v)}
          />
        </div>
      )}
    </div>
  );
}

function StatCard({
  icon: Icon,
  label,
  value,
  sub,
}: {
  icon: ComponentType<{ className?: string }>;
  label: string;
  value: string;
  sub: string;
}) {
  return (
    <div className="panel p-5">
      <div className="flex items-center gap-2 text-muted">
        <Icon className="h-4 w-4 text-gold" />
        <span className="eyebrow !text-muted">{label}</span>
      </div>
      <p className="mt-2.5 font-mono text-2xl font-semibold tabular-nums text-ink">{value}</p>
      <p className="mt-1 text-xs text-faint">{sub}</p>
    </div>
  );
}

function ChartCard({
  title,
  color,
  dataKey,
  data,
  fmt,
  percent = false,
  icon: Icon,
}: {
  title: string;
  color: string;
  dataKey: keyof Point;
  data: Point[];
  fmt: (v: number) => string;
  percent?: boolean;
  icon?: ComponentType<{ className?: string }>;
}) {
  const gid = useMemo(() => `grad-${String(dataKey)}`, [dataKey]);
  return (
    <div className="panel p-5">
      <div className="mb-3 flex items-center gap-2">
        {Icon && <Icon className="h-3.5 w-3.5 text-faint" />}
        <span className="eyebrow">{title}</span>
      </div>
      <div className="h-52">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={data} margin={{ top: 4, right: 6, left: -6, bottom: 0 }}>
            <defs>
              <linearGradient id={gid} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={color} stopOpacity={0.45} />
                <stop offset="100%" stopColor={color} stopOpacity={0} />
              </linearGradient>
            </defs>
            <XAxis
              dataKey="time"
              tick={{ fill: "#5c6477", fontSize: 10, fontFamily: "JetBrains Mono" }}
              tickLine={false}
              axisLine={{ stroke: "#232a38" }}
              minTickGap={48}
            />
            <YAxis
              tick={{ fill: "#5c6477", fontSize: 10, fontFamily: "JetBrains Mono" }}
              tickLine={false}
              axisLine={false}
              width={54}
              domain={percent ? [0, (max: number) => Math.max(100, Math.ceil(max / 20) * 20)] : [0, "auto"]}
              tickFormatter={(v) => fmt(Number(v))}
            />
            <Tooltip
              cursor={{ stroke: color, strokeOpacity: 0.3 }}
              contentStyle={{
                background: "#161b25",
                border: "1px solid #232a38",
                borderRadius: 8,
                fontSize: 12,
                fontFamily: "JetBrains Mono",
                color: "#e6e9ef",
              }}
              formatter={(v: number) => [fmt(v), title]}
            />
            <Area
              type="monotone"
              dataKey={dataKey}
              stroke={color}
              strokeWidth={2}
              fill={`url(#${gid})`}
              isAnimationActive={false}
              dot={false}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
