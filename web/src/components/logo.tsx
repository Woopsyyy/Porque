import { cn } from "@/lib/utils";

export function Logo({ size = 34, className }: { size?: number; className?: string }) {
  return (
    <div className={cn("flex items-center gap-2.5", className)}>
      <div className="relative">
        <div
          className="absolute -inset-1 rounded-full bg-gold/20 blur-md"
          aria-hidden
        />
        <img
          src="/mascot.png"
          alt="Porque"
          width={size}
          height={size}
          className="relative drop-shadow-[0_2px_6px_rgba(0,0,0,0.5)]"
        />
      </div>
      <div className="leading-none">
        <span className="font-display text-[1.35rem] font-extrabold tracking-tight text-ink">
          Porque
        </span>
        <span className="ml-0.5 text-[1.35rem] font-extrabold text-gold">.</span>
      </div>
    </div>
  );
}
