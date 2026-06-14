import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { Save, Folder, HardDrive } from "lucide-react";
import { api, ApiError } from "@/lib/api";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton, Spinner } from "@/components/ui/misc";

export default function SettingsPage() {
  const [serversPath, setServersPath] = useState("");

  const { isLoading } = useQuery({
    queryKey: ["settings"],
    queryFn: async () => {
      const data = await api.getSettings();
      setServersPath(data.servers_path);
      return data;
    },
  });

  const save = useMutation({
    mutationFn: () => api.updateSettings({ servers_path: serversPath }),
    onSuccess: () => {
      toast.success("Settings saved successfully");
    },
    onError: (e) => {
      toast.error(e instanceof ApiError ? e.message : "Failed to save settings");
    },
  });

  return (
    <div className="space-y-7">
      <PageHeader
        title="Settings"
        subtitle="Configure your host directories and automated storage properties."
      />

      {isLoading ? (
        <div className="space-y-4">
          <Skeleton className="h-40 w-full" />
          <Skeleton className="h-32 w-full" />
        </div>
      ) : (
        <div className="max-w-3xl space-y-6">
          {/* Main configuration settings */}
          <Card className="panel border border-border bg-surface/50 backdrop-blur-md">
            <CardHeader className="border-b border-border/50 pb-4">
              <CardTitle className="flex items-center gap-2 font-display text-lg font-bold text-ink">
                <HardDrive className="h-5 w-5 text-gold" />
                Storage configuration
              </CardTitle>
              <CardDescription className="text-xs text-muted">
                Configure directory paths and storage options on the host machine.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-6 pt-5">
              <div className="space-y-2">
                <Label htmlFor="servers-path" className="flex items-center gap-1.5">
                  <Folder className="h-3.5 w-3.5 text-muted" />
                  Minecraft Servers Path (Host)
                </Label>
                <Input
                  id="servers-path"
                  value={serversPath}
                  onChange={(e) => setServersPath(e.target.value)}
                  placeholder="e.g. C:/Minecraft/Servers"
                  className="font-mono text-sm"
                />
                <p className="text-xs text-muted">
                  This is the absolute path on your host machine where server directories are created.
                </p>
              </div>

              <div className="flex justify-end pt-2">
                <Button
                  variant="primary"
                  onClick={() => save.mutate()}
                  disabled={save.isPending}
                  className="flex items-center gap-2"
                >
                  {save.isPending ? <Spinner className="h-4 w-4" /> : <Save className="h-4 w-4" />}
                  Save Settings
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  );
}
