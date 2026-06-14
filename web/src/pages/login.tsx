import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { ApiError } from "@/lib/api";
import { useAuth } from "@/providers/auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/misc";

export default function LoginPage() {
  const { login } = useAuth();
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      await login(username, password);
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Sign in failed");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="relative grid min-h-screen place-items-center overflow-hidden p-6">
      {/* Hero glow */}
      <div
        aria-hidden
        className="pointer-events-none absolute left-1/2 top-[28%] h-[420px] w-[520px] -translate-x-1/2 rounded-full bg-gold/12 blur-[120px]"
      />

      <div className="relative w-full max-w-sm animate-fade-up">
        {/* Mascot crest */}
        <div className="mb-7 flex flex-col items-center text-center">
          <div className="relative mb-4">
            <div className="absolute -inset-3 rounded-full bg-gold/20 blur-xl" aria-hidden />
            <img
              src="/mascot.png"
              alt="Porque"
              width={84}
              height={84}
              className="relative drop-shadow-[0_6px_16px_rgba(0,0,0,0.6)]"
            />
          </div>
          <h1 className="font-display text-4xl font-extrabold tracking-tight text-ink">
            Porque<span className="text-gold">.</span>
          </h1>
          <p className="eyebrow mt-2">Minecraft Control Room</p>
        </div>

        <form onSubmit={submit} className="panel space-y-4 p-6">
          <div className="space-y-1.5">
            <Label htmlFor="username">Username</Label>
            <Input
              id="username"
              autoComplete="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoFocus
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="password">Password</Label>
            <Input
              id="password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>
          <Button type="submit" variant="primary" size="lg" className="w-full" disabled={loading}>
            {loading && <Spinner className="h-4 w-4" />}
            {loading ? "Signing in…" : "Enter the Control Room"}
          </Button>
        </form>

        <p className="mt-5 text-center font-mono text-[0.7rem] text-faint">
          self-hosted · docker-native · playit ready
        </p>
      </div>
    </div>
  );
}
