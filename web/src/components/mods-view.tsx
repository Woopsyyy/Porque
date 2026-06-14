import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Package, Trash2, Upload } from "lucide-react";
import { toast } from "sonner";
import { api, ApiError, type Server } from "@/lib/api";
import { formatBytes } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Skeleton, Spinner } from "@/components/ui/misc";
import { cn } from "@/lib/utils";

export function ModsView({ server }: { server: Server }) {
  const qc = useQueryClient();
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);

  const { data, isLoading } = useQuery({
    queryKey: ["mods", server.id],
    queryFn: () => api.listMods(server.id),
  });
  const folder = data?.folder ?? (server.server_type === "PAPER" ? "plugins" : "mods");
  const mods = data?.mods ?? [];

  const upload = useMutation({
    mutationFn: (files: File[]) => api.uploadMods(server.id, files),
    onSuccess: (res) => {
      toast.success(`Uploaded — ${res.mods.length} file(s) in ${res.folder}/`);
      qc.setQueryData(["mods", server.id], res);
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Upload failed"),
  });
  const del = useMutation({
    mutationFn: (name: string) => api.deleteMod(server.id, name),
    onSuccess: () => {
      toast.success("Removed");
      qc.invalidateQueries({ queryKey: ["mods", server.id] });
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : "Delete failed"),
  });

  const handleFiles = (fileList: FileList | null) => {
    if (!fileList) return;
    const files = Array.from(fileList).filter((f) => f.name.toLowerCase().endsWith(".jar"));
    if (files.length === 0) {
      toast.error("Only .jar files are supported");
      return;
    }
    upload.mutate(files);
  };

  return (
    <div className="w-full space-y-4">
      <div
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDragging(false);
          handleFiles(e.dataTransfer.files);
        }}
        onClick={() => inputRef.current?.click()}
        className={cn(
          "flex cursor-pointer flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed px-6 py-10 text-center transition-colors",
          dragging ? "border-gold bg-gold/5" : "border-border bg-surface/40 hover:border-faint",
        )}
      >
        <input
          ref={inputRef}
          type="file"
          accept=".jar"
          multiple
          className="hidden"
          onChange={(e) => {
            handleFiles(e.target.files);
            e.target.value = "";
          }}
        />
        {upload.isPending ? (
          <Spinner className="h-6 w-6 text-gold" />
        ) : (
          <Upload className="h-6 w-6 text-gold" />
        )}
        <p className="text-sm text-ink">
          {upload.isPending ? (
            "Uploading…"
          ) : (
            <>
              Drop <span className="font-mono">.jar</span> files here, or click to browse
            </>
          )}
        </p>
        <p className="font-mono text-[0.68rem] text-faint">
          installs into <span className="text-muted">/{folder}</span> · restart to load
        </p>
      </div>

      {isLoading ? (
        <Skeleton className="h-20 w-full" />
      ) : mods.length > 0 ? (
        <div className="grid gap-3 [grid-template-columns:repeat(auto-fill,minmax(280px,1fr))]">
          {mods.map((m) => (
            <div key={m.name} className="panel flex flex-col justify-between p-4 min-w-0 relative group hover:border-gold/30 transition-colors">
              <div className="flex items-start gap-2.5 min-w-0">
                <Package className="h-4.5 w-4.5 shrink-0 text-gold mt-0.5" />
                <div className="min-w-0 flex-1">
                  <p className="truncate font-mono text-xs font-semibold text-ink" title={m.name}>
                    {m.name}
                  </p>
                  <p className="font-mono text-[0.68rem] text-faint mt-1">{formatBytes(m.size)}</p>
                </div>
              </div>
              <div className="flex justify-end mt-3 border-t border-border/30 pt-2">
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs text-muted hover:text-danger hover:bg-danger/5"
                  onClick={() => del.mutate(m.name)}
                  disabled={del.isPending}
                >
                  <Trash2 className="h-3.5 w-3.5 mr-1" />
                  Remove
                </Button>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="panel grid h-28 place-items-center text-sm text-faint">
          No {folder} installed yet.
        </div>
      )}
    </div>
  );
}
