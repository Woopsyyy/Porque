import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError, type ServerType } from "@/lib/api";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

const TYPES: ServerType[] = ["VANILLA", "PAPER", "FABRIC", "FORGE"];
const VERSIONS = [
  "1.21",
  "1.20.6",
  "1.20.4",
  "1.20.1",
  "1.19.4",
  "1.19.2",
  "1.18.2",
  "1.16.5",
  "1.12.2",
];

export function CreateServerDialog() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [type, setType] = useState<ServerType>("PAPER");
  const [versionSelect, setVersionSelect] = useState("1.20.4");
  const [version, setVersion] = useState("1.20.4");
  const [loader, setLoader] = useState("");
  const [memory, setMemory] = useState("2048");

  const reset = () => {
    setName("");
    setType("PAPER");
    setVersionSelect("1.20.4");
    setVersion("1.20.4");
    setLoader("");
    setMemory("2048");
  };

  const mutation = useMutation({
    mutationFn: () =>
      api.createServer({
        name: name.trim(),
        type,
        version: version.trim(),
        loader_version: loader.trim() || undefined,
        memory_mb: Number(memory),
      }),
    onSuccess: (srv) => {
      qc.invalidateQueries({ queryKey: ["servers"] });
      toast.success(`Server “${srv.name}” created`);
      setOpen(false);
      reset();
      navigate(`/servers/${srv.id}`);
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Could not create server"),
  });

  const showLoader = type === "FABRIC" || type === "FORGE";

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="primary">
          <Plus className="h-4 w-4" />
          New server
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New server</DialogTitle>
          <DialogDescription>
            Provisions a server folder and prepares the server. It won’t start until you press Start.
          </DialogDescription>
        </DialogHeader>

        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            mutation.mutate();
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              placeholder="survival"
              value={name}
              onChange={(e) => setName(e.target.value)}
              autoFocus
            />
            <p className="font-mono text-[0.68rem] text-faint">
              1–64 characters · letters, numbers, spaces, special chars
            </p>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <Label>Type</Label>
              <Select value={type} onValueChange={(v) => setType(v as ServerType)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TYPES.map((t) => (
                    <SelectItem key={t} value={t}>
                      {t}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label>Version</Label>
              <Select
                value={versionSelect}
                onValueChange={(v) => {
                  setVersionSelect(v);
                  if (v !== "custom") {
                    setVersion(v);
                  } else {
                    setVersion("");
                  }
                }}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {VERSIONS.map((v) => (
                    <SelectItem key={v} value={v}>
                      {v}
                    </SelectItem>
                  ))}
                  <SelectItem value="custom">Custom version...</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          {versionSelect === "custom" && (
            <div className="space-y-1.5">
              <Label htmlFor="version">Custom Version</Label>
              <Input
                id="version"
                placeholder="e.g. 1.20.4, 24w14a"
                value={version}
                onChange={(e) => setVersion(e.target.value)}
              />
            </div>
          )}

          {showLoader && (
            <div className="space-y-1.5">
              <Label htmlFor="loader">
                {type === "FABRIC" ? "Fabric loader" : "Forge"} version (optional)
              </Label>
              <Input
                id="loader"
                placeholder="latest"
                value={loader}
                onChange={(e) => setLoader(e.target.value)}
              />
            </div>
          )}

          <div className="space-y-1.5">
            <Label htmlFor="memory">Memory (MB)</Label>
            <Input
              id="memory"
              type="number"
              min={512}
              step={256}
              value={memory}
              onChange={(e) => setMemory(e.target.value)}
            />
            <p className="font-mono text-[0.68rem] text-faint">CPU is auto-allocated from RAM.</p>
          </div>

          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" variant="primary" disabled={mutation.isPending}>
              {mutation.isPending && <Spinner className="h-4 w-4" />}
              Create server
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
