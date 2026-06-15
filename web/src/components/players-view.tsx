import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2, UserPlus, Users } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError, type Player, type Server } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton, Spinner } from "@/components/ui/misc";

// headURL returns a Minecraft head avatar for a player. Bedrock (Geyser) names
// carry a Floodgate prefix (".") which we strip before the skin lookup.
function headURL(p: Player): string {
  const name = p.edition === "bedrock" ? p.name.replace(/^\.+/, "") : p.name;
  return `https://mc-heads.net/avatar/${encodeURIComponent(name || "steve")}/64`;
}

function EditionBadge({ edition }: { edition: Player["edition"] }) {
  const isBedrock = edition === "bedrock";
  return (
    <span
      className={
        "rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide " +
        (isBedrock
          ? "bg-[#5BA8FF]/15 text-[#5BA8FF]"
          : "bg-gold/15 text-gold")
      }
    >
      {isBedrock ? "Bedrock" : "Java"}
    </span>
  );
}

function PlayerCard({
  player,
  onRemove,
  removing,
}: {
  player: Player;
  onRemove?: () => void;
  removing?: boolean;
}) {
  return (
    <div className="panel group relative flex flex-col items-center gap-3 p-4 text-center transition-all duration-300 hover:border-gold/30">
      <img
        src={headURL(player)}
        alt={`${player.name} head`}
        width={64}
        height={64}
        loading="lazy"
        onError={(e) => {
          const img = e.currentTarget;
          if (!img.dataset.fallback) {
            img.dataset.fallback = "1";
            img.src = "https://mc-heads.net/avatar/steve/64";
          }
        }}
        className="h-16 w-16 rounded-md border border-border [image-rendering:pixelated]"
      />
      {/* "button part": username + edition badge */}
      <div className="flex w-full flex-col items-center gap-1.5">
        <span className="max-w-full truncate font-display text-sm font-semibold text-ink" title={player.name}>
          {player.name}
        </span>
        <EditionBadge edition={player.edition} />
      </div>

      {onRemove && (
        <button
          type="button"
          aria-label={`Remove ${player.name} from whitelist`}
          onClick={onRemove}
          disabled={removing}
          className="absolute inset-0 flex items-center justify-center rounded-xl bg-bg/70 opacity-0 backdrop-blur-sm transition-opacity duration-300 hover:opacity-100 focus-visible:opacity-100 motion-reduce:transition-none disabled:cursor-not-allowed"
        >
          <span className="flex h-11 w-11 items-center justify-center rounded-full bg-danger/15 text-danger">
            {removing ? <Spinner className="h-4 w-4" /> : <Trash2 className="h-5 w-5" />}
          </span>
        </button>
      )}
    </div>
  );
}

function EmptyState({ children }: { children: React.ReactNode }) {
  return (
    <div className="panel grid place-items-center px-4 py-10 text-sm text-faint">{children}</div>
  );
}

export function PlayersView({ server }: { server: Server }) {
  const qc = useQueryClient();
  const running = server.state === "running";
  const [name, setName] = useState("");

  const online = useQuery({
    queryKey: ["players-online", server.id],
    queryFn: () => api.listOnlinePlayers(server.id),
    refetchInterval: running ? 5000 : false,
    enabled: running,
  });

  const whitelist = useQuery({
    queryKey: ["whitelist", server.id],
    queryFn: () => api.getWhitelist(server.id),
    refetchInterval: 10000,
  });

  const onErr = (e: unknown) =>
    toast.error(e instanceof ApiError ? e.message : "Action failed");
  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["whitelist", server.id] });
    qc.invalidateQueries({ queryKey: ["players-online", server.id] });
  };

  const add = useMutation({
    mutationFn: (n: string) => api.addToWhitelist(server.id, n),
    onSuccess: () => {
      toast.success("Player whitelisted");
      setName("");
      invalidate();
    },
    onError: onErr,
  });

  const [removingName, setRemovingName] = useState<string | null>(null);
  const remove = useMutation({
    mutationFn: (n: string) => api.removeFromWhitelist(server.id, n),
    onMutate: (n) => setRemovingName(n),
    onSuccess: () => {
      toast.success("Player removed");
      invalidate();
    },
    onError: onErr,
    onSettled: () => setRemovingName(null),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (trimmed) add.mutate(trimmed);
  };

  return (
    <div className="space-y-6">
      {/* Online now */}
      <section className="space-y-3">
        <div className="flex items-center gap-2">
          <Users className="h-4 w-4 text-gold" />
          <span className="eyebrow">Online now</span>
        </div>
        {!running ? (
          <EmptyState>Start the server to see who's online.</EmptyState>
        ) : online.isLoading ? (
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-36 w-full" />
            ))}
          </div>
        ) : (online.data?.length ?? 0) === 0 ? (
          <EmptyState>No players online right now.</EmptyState>
        ) : (
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
            {online.data!.map((p) => (
              <PlayerCard key={`online-${p.name}`} player={p} />
            ))}
          </div>
        )}
      </section>

      {/* Whitelist */}
      <section className="space-y-3">
        <div className="flex items-center gap-2">
          <UserPlus className="h-4 w-4 text-gold" />
          <span className="eyebrow">Whitelist</span>
        </div>

        <form onSubmit={submit} className="flex items-center gap-2">
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Minecraft username"
            aria-label="Minecraft username"
            maxLength={32}
            disabled={add.isPending}
          />
          <Button type="submit" variant="primary" disabled={add.isPending || !name.trim()}>
            {add.isPending ? <Spinner className="h-4 w-4" /> : <UserPlus className="h-4 w-4" />}
            Add Player
          </Button>
        </form>

        {whitelist.isLoading ? (
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-36 w-full" />
            ))}
          </div>
        ) : (whitelist.data?.length ?? 0) === 0 ? (
          <EmptyState>The whitelist is empty. Add a player above.</EmptyState>
        ) : (
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
            {whitelist.data!.map((p) => (
              <PlayerCard
                key={`wl-${p.name}`}
                player={p}
                onRemove={() => remove.mutate(p.name)}
                removing={removingName === p.name}
              />
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
