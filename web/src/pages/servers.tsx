import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { ServerCard } from "@/components/server-card";
import { CreateServerDialog } from "@/components/create-server-dialog";
import { ImportServerDialog } from "@/components/import-server-dialog";
import { Skeleton } from "@/components/ui/misc";

export default function ServersPage() {
  const { data: servers, isLoading } = useQuery({
    queryKey: ["servers"],
    queryFn: api.listServers,
    refetchInterval: 5000,
  });

  return (
    <div className="space-y-7">
      <PageHeader
        title="Servers"
        subtitle="Provision, run, and watch over your Minecraft worlds."
        action={
          <div className="flex items-center gap-2">
            <ImportServerDialog />
            <CreateServerDialog />
          </div>
        }
      />

      {isLoading ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-44" />
          ))}
        </div>
      ) : servers && servers.length > 0 ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {servers.map((s) => (
            <ServerCard key={s.id} server={s} />
          ))}
        </div>
      ) : (
        <EmptyState />
      )}
    </div>
  );
}

function EmptyState() {
  return (
    <div className="panel flex flex-col items-center gap-4 px-6 py-16 text-center">
      <img src="/mascot.png" alt="" width={72} height={72} className="opacity-90" />
      <div>
        <h3 className="font-display text-xl font-bold text-ink">No servers yet</h3>
        <p className="mx-auto mt-1 max-w-sm text-sm text-muted">
          Spin up your first Minecraft server — Vanilla, Paper, Fabric, or Forge — in a few seconds.
        </p>
      </div>
      <div className="flex gap-3">
        <ImportServerDialog />
        <CreateServerDialog />
      </div>
    </div>
  );
}
