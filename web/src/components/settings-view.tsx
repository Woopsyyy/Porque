import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ImagePlus, Save } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError, type Server } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/misc";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { formatBytes } from "@/lib/format";
import { cn } from "@/lib/utils";

const DIFFICULTIES = ["peaceful", "easy", "normal", "hard"];

function ramLabel(mb: number): string {
  return mb >= 1024 ? `${(mb / 1024).toFixed(mb % 1024 ? 1 : 0)} GB` : `${mb} MB`;
}

export function SettingsView({ server }: { server: Server }) {
  const qc = useQueryClient();
  const { data: system } = useQuery({ queryKey: ["system"], queryFn: api.getSystem });
  const hostMB = system?.ram_total_bytes ? Math.floor(system.ram_total_bytes / 1048576) : 8192;

  const [motd, setMotd] = useState(server.motd);
  const [difficulty, setDifficulty] = useState(server.difficulty);
  const [cracked, setCracked] = useState(!server.online_mode);
  const [memory, setMemory] = useState(server.memory_mb);

  useEffect(() => {
    setMotd(server.motd);
    setDifficulty(server.difficulty);
    setCracked(!server.online_mode);
    setMemory(server.memory_mb);
  }, [server.id]); // eslint-disable-line react-hooks/exhaustive-deps

  // RAM zones relative to host RAM.
  const recommendedMax = Math.max(1024, Math.floor(hostMB * 0.5));
  const cautionMax = Math.max(recommendedMax, Math.floor(hostMB * 0.8));
  const zone = memory <= recommendedMax ? "ok" : memory <= cautionMax ? "warn" : "danger";
  const zoneColor = zone === "ok" ? "#34D399" : zone === "warn" ? "#FBBF24" : "#F87171";
  const zoneLabel =
    zone === "ok"
      ? "Recommended"
      : zone === "warn"
        ? "Not recommended"
        : "Too high — may starve the host";

  const dirty =
    motd !== server.motd ||
    difficulty !== server.difficulty ||
    cracked !== !server.online_mode ||
    memory !== server.memory_mb;

  const save = useMutation({
    mutationFn: () =>
      api.updateServerSettings(server.id, {
        difficulty,
        online_mode: !cracked,
        motd: motd.trim() || "A Minecraft Server",
        memory_mb: memory,
      }),
    onSuccess: () => {
      toast.success("Settings saved — restart the server to apply");
      qc.invalidateQueries({ queryKey: ["server", server.id] });
      qc.invalidateQueries({ queryKey: ["servers"] });
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Save failed"),
  });

  return (
    <div className="max-w-2xl space-y-5">
      <IconCard server={server} />

      {/* server.properties */}
      <div className="panel space-y-5 p-6">
        <span className="eyebrow">Server properties</span>

        <div className="space-y-1.5">
          <Label htmlFor="motd">Server name (MOTD)</Label>
          <Input id="motd" value={motd} onChange={(e) => setMotd(e.target.value)} maxLength={59} />
          <p className="font-mono text-[0.68rem] text-faint">
            Shown in the multiplayer server list.
          </p>
        </div>

        <div className="grid gap-5 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label>Difficulty</Label>
            <Select value={difficulty} onValueChange={setDifficulty}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {DIFFICULTIES.map((d) => (
                  <SelectItem key={d} value={d} className="capitalize">
                    {d}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label>Cracked accounts</Label>
            <div className="flex h-9 items-center gap-3">
              <button
                type="button"
                role="switch"
                aria-checked={cracked}
                onClick={() => setCracked((c) => !c)}
                className={cn(
                  "relative h-6 w-11 shrink-0 rounded-full transition-colors",
                  cracked ? "bg-gold" : "border border-border bg-surface-2",
                )}
              >
                <span
                  className={cn(
                    "absolute top-0.5 left-0.5 h-5 w-5 rounded-full bg-bg transition-transform",
                    cracked ? "translate-x-5 bg-ink" : "translate-x-0",
                  )}
                />
              </button>
              <span className="text-sm text-muted">
                {cracked ? "Offline mode — non-premium allowed" : "Online mode (premium only)"}
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* Resource allocation */}
      <div className="panel space-y-4 p-6">
        <span className="eyebrow">Resource allocation</span>
        <div>
          <div className="flex items-center justify-between">
            <Label>RAM allocation</Label>
            <span className="font-mono text-sm font-semibold" style={{ color: zoneColor }}>
              {ramLabel(memory)}
            </span>
          </div>
          <input
            type="range"
            min={512}
            max={hostMB}
            step={256}
            value={Math.min(memory, hostMB)}
            onChange={(e) => setMemory(Number(e.target.value))}
            className="mt-3 w-full cursor-pointer"
            style={{ accentColor: zoneColor }}
          />
          <div className="mt-1.5 flex items-center justify-between font-mono text-[0.65rem] text-faint">
            <span>512 MB</span>
            <span style={{ color: zoneColor }}>{zoneLabel}</span>
            <span>{formatBytes(hostMB * 1048576)} host</span>
          </div>
          <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 font-mono text-[0.65rem] text-muted">
            <Legend color="#34D399" label={`Recommended ≤ ${ramLabel(recommendedMax)}`} />
            <Legend color="#FBBF24" label={`Caution ≤ ${ramLabel(cautionMax)}`} />
            <Legend color="#F87171" label="Too high" />
          </div>
          <p className="mt-3 font-mono text-[0.68rem] text-faint">
            CPU is auto-allocated from RAM (≈1 core per 2 GB).
          </p>
        </div>
      </div>

      <div className="flex items-center gap-3">
        <Button variant="primary" onClick={() => save.mutate()} disabled={!dirty || save.isPending}>
          {save.isPending ? <Spinner className="h-4 w-4" /> : <Save className="h-4 w-4" />}
          Save settings
        </Button>
        <span className="text-xs text-faint">Changes apply on the next start.</span>
      </div>
    </div>
  );
}

function IconCard({ server }: { server: Server }) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [version, setVersion] = useState(0);
  const [hasIcon, setHasIcon] = useState(true);

  const upload = useMutation({
    mutationFn: (file: File) => api.uploadIcon(server.id, file),
    onSuccess: () => {
      toast.success("Server icon updated");
      setHasIcon(true);
      setVersion((v) => v + 1);
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Icon upload failed"),
  });

  const pick = (files: FileList | null) => {
    const f = files?.[0];
    if (f) upload.mutate(f);
  };

  return (
    <div className="panel flex items-center gap-5 p-6">
      <div className="grid h-16 w-16 shrink-0 place-items-center overflow-hidden rounded-md border border-border bg-bg/60">
        {hasIcon ? (
          <img
            src={`${api.iconUrl(server.id)}?v=${version}`}
            alt="Server icon"
            width={64}
            height={64}
            className="h-16 w-16 [image-rendering:pixelated]"
            onError={() => setHasIcon(false)}
          />
        ) : (
          <ImagePlus className="h-6 w-6 text-faint" />
        )}
      </div>
      <div className="min-w-0">
        <span className="eyebrow">Server icon</span>
        <p className="mt-1 text-sm text-muted">
          Any image — auto-cropped &amp; resized to a crisp 64×64 PNG. Restart to show it in the
          server list.
        </p>
        <input
          ref={inputRef}
          type="file"
          accept="image/*"
          className="hidden"
          onChange={(e) => {
            pick(e.target.files);
            e.target.value = "";
          }}
        />
        <Button
          variant="outline"
          size="sm"
          className="mt-3"
          onClick={() => inputRef.current?.click()}
          disabled={upload.isPending}
        >
          {upload.isPending ? <Spinner className="h-4 w-4" /> : <ImagePlus className="h-4 w-4" />}
          Upload icon
        </Button>
      </div>
    </div>
  );
}

function Legend({ color, label }: { color: string; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className="h-2 w-2 rounded-full" style={{ backgroundColor: color }} />
      {label}
    </span>
  );
}
