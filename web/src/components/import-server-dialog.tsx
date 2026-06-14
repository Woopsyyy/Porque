import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Folder, Upload } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/misc";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

export function ImportServerDialog() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [hostPath, setHostPath] = useState("");

  const reset = () => {
    setName("");
    setHostPath("");
  };

  const mutation = useMutation({
    mutationFn: () => {
      if (!name.trim()) {
        throw new Error("Server name is required");
      }
      if (!hostPath.trim()) {
        throw new Error("Minecraft Folder Path on host is required");
      }
      return api.importServer({
        name: name.trim(),
        host_path: hostPath.trim(),
        type: "VANILLA",
        version: "latest",
      });
    },
    onSuccess: (srv) => {
      qc.invalidateQueries({ queryKey: ["servers"] });
      toast.success(`Server “${srv.name}” connected successfully`);
      setOpen(false);
      reset();
      navigate(`/servers/${srv.id}`);
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : e instanceof Error ? e.message : "Import failed"),
  });

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" className="flex items-center gap-1.5">
          <Upload className="h-4 w-4" />
          Import Server
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Import server</DialogTitle>
          <DialogDescription>
            Specify an existing Minecraft server folder on your host machine to link and manage it.
          </DialogDescription>
        </DialogHeader>

        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            mutation.mutate();
          }}
        >
          {/* Server Name */}
          <div className="space-y-1.5">
            <Label htmlFor="import-name">Name</Label>
            <Input
              id="import-name"
              placeholder="my-imported-server"
              value={name}
              onChange={(e) => setName(e.target.value)}
              autoFocus
            />
            <p className="font-mono text-[0.68rem] text-faint">
              1–64 characters · letters, numbers, spaces, special chars
            </p>
          </div>

          {/* Host Path Input */}
          <div className="space-y-1.5">
            <Label htmlFor="host-path" className="flex items-center gap-1">
              <Folder className="h-3.5 w-3.5 text-muted" />
              Minecraft Folder Path (Host)
            </Label>
            <div className="flex gap-2">
              <Input
                id="host-path"
                placeholder="e.g. C:/MinecraftServers/myserver"
                value={hostPath}
                onChange={(e) => setHostPath(e.target.value)}
                className="font-mono text-sm flex-1"
              />
              <Button
                type="button"
                variant="outline"
                onClick={async () => {
                  try {
                    const dir = await api.selectFolder();
                    if (dir) {
                      setHostPath(dir);
                      if (!name.trim()) {
                        const parts = dir.split(/[/\\]/);
                        const last = parts.filter(Boolean).pop();
                        if (last) {
                          setName(last);
                        }
                      }
                    }
                  } catch (err) {
                    toast.error("Failed to open folder dialog");
                  }
                }}
              >
                Browse...
              </Button>
            </div>
            <p className="font-mono text-[0.68rem] text-faint">
              Absolute host path where this Minecraft server files live.
            </p>
          </div>

          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" variant="primary" disabled={mutation.isPending || !name.trim() || !hostPath.trim()}>
              {mutation.isPending && <Spinner className="h-4 w-4" />}
              Import server
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
