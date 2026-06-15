import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowDownToLine, Cable, Terminal, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { toLines } from "@/lib/ansi";
import { api } from "@/lib/api";
import { useWebSocket } from "@/lib/ws";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

const MAX_LINES = 2000;

function mcTone(line: string): string {
  if (/\b(ERROR|FATAL|severe)\b/i.test(line)) return "text-danger";
  if (/\bWARN\b/i.test(line)) return "text-warn";
  if (/Done \(|For help, type/i.test(line)) return "text-running";
  return "text-ink/85";
}

function playitTone(line: string): string {
  if (/error|fail|invalid/i.test(line)) return "text-danger";
  if (/warn/i.test(line)) return "text-warn";
  if (/online|registered|running|tunnel/i.test(line)) return "text-running";
  return "text-gold/75";
}

function LogStream({
  path,
  enabled,
  idle,
  tone,
  onSendCommand,
}: {
  path: string;
  enabled: boolean;
  idle: string;
  tone: (line: string) => string;
  onSendCommand?: (cmd: string) => Promise<void>;
}) {
  const [lines, setLines] = useState<string[]>([]);
  const [follow, setFollow] = useState(true);
  const [command, setCommand] = useState("");
  const [sending, setSending] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setLines([]);
  }, [path, enabled]);

  const status = useWebSocket(enabled ? path : null, (data) => {
    const next = toLines(data);
    if (next.length) setLines((prev) => [...prev, ...next].slice(-MAX_LINES));
  });

  useLayoutEffect(() => {
    if (follow && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [lines, follow]);

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!command.trim() || sending || !onSendCommand) return;
    setSending(true);
    try {
      let cmd = command.trim();
      if (cmd.startsWith("/")) {
        cmd = cmd.substring(1);
      }
      await onSendCommand(cmd);
      setCommand("");
    } catch {
      // toast is handled in parent
    } finally {
      setSending(false);
    }
  };

  return (
    <div className="panel overflow-hidden">
      <div className="flex items-center justify-between border-b border-border px-4 py-2.5">
        <span
          className={cn(
            "inline-flex items-center gap-1.5 font-mono text-[0.68rem]",
            status === "open" ? "text-running" : "text-faint",
          )}
        >
          <span
            className={cn(
              "h-1.5 w-1.5 rounded-full",
              status === "open" ? "animate-pulsedot bg-running" : "bg-faint",
            )}
          />
          {status === "open" ? "live" : enabled ? "connecting" : "offline"}
        </span>
        <div className="flex items-center gap-1">
          <Button
            variant={follow ? "outline" : "ghost"}
            size="sm"
            onClick={() => setFollow((f) => !f)}
            title="Auto-scroll"
          >
            <ArrowDownToLine className="h-3.5 w-3.5" />
            Follow
          </Button>
          <Button variant="ghost" size="sm" onClick={() => setLines([])} title="Clear">
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>
      <div
        ref={scrollRef}
        onWheel={() => setFollow(false)}
        className="console-scroll h-[420px] overflow-y-auto bg-[#070a0f] p-4 font-mono text-[0.78rem] leading-[1.55]"
      >
        {!enabled ? (
          <p className="text-faint">{idle}</p>
        ) : lines.length === 0 ? (
          <p className="text-faint">Waiting for output…</p>
        ) : (
          lines.map((line, i) => (
            <div key={i} className={cn("whitespace-pre-wrap break-words", tone(line))}>
              {line}
            </div>
          ))
        )}
      </div>

      {onSendCommand && enabled && (
        <form onSubmit={handleSend} className="flex items-center border-t border-border bg-[#0b0f19] px-4 py-2">
          <span className="mr-2 font-mono text-xs font-semibold text-gold">&gt;</span>
          <input
            type="text"
            value={command}
            onChange={(e) => setCommand(e.target.value)}
            disabled={sending}
            placeholder="Type a server command (e.g. /help, /list, /op)..."
            className="flex-1 bg-transparent py-1 font-mono text-xs text-ink placeholder-faint outline-none"
          />
          {sending && (
            <span className="ml-2 h-3.5 w-3.5 animate-spin rounded-full border-2 border-gold border-t-transparent" />
          )}
        </form>
      )}
    </div>
  );
}

export function ConsoleView({ serverId, running }: { serverId: string; running: boolean }) {
  const { data: tunnels } = useQuery({
    queryKey: ["tunnels", serverId],
    queryFn: () => api.getTunnels(serverId),
    refetchInterval: 8000,
  });
  const tunnelActive = (tunnels?.length ?? 0) > 0;

  const handleSendCommand = async (cmd: string) => {
    try {
      const resp = await api.sendServerCommand(serverId, cmd);
      if (resp) {
        toast.success(resp);
      } else {
        toast.success("Command executed successfully");
      }
    } catch (err: any) {
      toast.error(err?.message || "Failed to execute command");
      throw err;
    }
  };

  return (
    <Tabs defaultValue="mc">
      <TabsList>
        <TabsTrigger value="mc">
          <Terminal className="h-3.5 w-3.5" />
          Minecraft
        </TabsTrigger>
        <TabsTrigger value="playit">
          <Cable className="h-3.5 w-3.5" />
          Playit
        </TabsTrigger>
      </TabsList>

      <TabsContent value="mc">
        <LogStream
          path={`/ws/logs/${serverId}`}
          enabled={running}
          idle="Server is offline. Start it to stream the console."
          tone={mcTone}
          onSendCommand={handleSendCommand}
        />
      </TabsContent>
      <TabsContent value="playit">
        <LogStream
          path={`/ws/playit-logs/${serverId}`}
          enabled={running && tunnelActive}
          idle="Attach a tunnel to stream the Playit agent log."
          tone={playitTone}
        />
      </TabsContent>
    </Tabs>
  );
}
