import type { ReactNode } from "react";
import { NavLink } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { Boxes, Cable, Settings, LogOut, Terminal } from "lucide-react";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Logo } from "@/components/logo";

const NAV = [
  { to: "/", label: "Servers", icon: Boxes, end: true },
  { to: "/tunnels", label: "Tunnels", icon: Cable, end: false },
  { to: "/logs", label: "Logs", icon: Terminal, end: false },
  { to: "/settings", label: "Settings", icon: Settings, end: false },
];

export function AppShell({ children }: { children: ReactNode }) {
  const { data: servers } = useQuery({
    queryKey: ["servers"],
    queryFn: api.listServers,
    refetchInterval: 5000,
  });

  const total = servers?.length ?? 0;
  const running = servers?.filter((s) => s.state === "running").length ?? 0;

  return (
    <div className="flex h-screen w-screen overflow-hidden select-none">
      {/* Sidebar */}
      <aside className="sticky top-0 hidden h-screen w-60 shrink-0 flex-col border-r border-border bg-surface/40 backdrop-blur-sm md:flex">
        <div className="flex h-16 items-center border-b border-border px-5">
          <Logo />
        </div>

        <nav className="flex flex-1 flex-col gap-1 p-3">
          <p className="eyebrow px-3 pb-2 pt-2">Control</p>
          {NAV.map(({ to, label, icon: Icon, end }) => (
            <NavLink
              key={to}
              to={to}
              end={end}
              className={({ isActive }) =>
                cn(
                  "group relative flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                  isActive
                    ? "bg-gold/10 text-gold"
                    : "text-muted hover:bg-surface-2 hover:text-ink",
                )
              }
            >
              {({ isActive }) => (
                <>
                  <span
                    className={cn(
                      "absolute left-0 top-1/2 h-5 w-0.5 -translate-y-1/2 rounded-full bg-gold transition-opacity",
                      isActive ? "opacity-100" : "opacity-0",
                    )}
                  />
                  <Icon className="h-4 w-4" />
                  {label}
                </>
              )}
            </NavLink>
          ))}
        </nav>

        <div className="border-t border-border p-3 flex flex-col gap-2">
          <button
            onClick={() => api.quitApp()}
            className="flex items-center gap-2.5 rounded-md px-3 py-2 text-xs font-medium text-muted hover:bg-danger/10 hover:text-danger transition-colors text-left w-full"
            title="Quit the application completely"
          >
            <LogOut className="h-3.5 w-3.5 text-faint" />
            Quit Application
          </button>
          
          <div className="flex items-center justify-between font-mono text-[0.68rem] text-faint px-3 pt-1 border-t border-border/20">
            <span>{total} servers</span>
            <span className="inline-flex items-center gap-1.5">
              <span className="h-1.5 w-1.5 animate-pulsedot rounded-full bg-running" />
              {running} live
            </span>
          </div>
        </div>
      </aside>

      {/* Main column */}
      <div className="flex min-w-0 flex-1 flex-col h-full overflow-hidden">
        <header className="sticky top-0 z-30 flex h-16 shrink-0 items-center justify-between border-b border-border bg-bg/80 px-6 backdrop-blur-md">
          <div className="flex items-center gap-3 md:hidden">
            <Logo size={28} />
          </div>
          <div className="hidden items-center gap-2 md:flex">
            <span className="eyebrow">Minecraft Control Room</span>
          </div>

          <div className="flex items-center gap-4">
            <span className="hidden items-center gap-2 rounded-full border border-border bg-surface/60 px-3 py-1 font-mono text-xs text-muted sm:inline-flex">
              <span className="h-1.5 w-1.5 animate-pulsedot rounded-full bg-running" />
              {running} running
            </span>
          </div>
        </header>

        <main className="flex-1 overflow-y-auto px-6 py-7">
          <div className="mx-auto w-full max-w-6xl animate-fade-up">{children}</div>
        </main>
      </div>
    </div>
  );
}
