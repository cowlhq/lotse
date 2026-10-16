import { useMutation, useQueryClient } from "@tanstack/react-query";
import { FileUp, Minus, Pencil, Plus, Upload } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { applyDocument, parseDocument, type ApplyChange, type ApplyResult } from "@/api";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ago } from "@/lib/format";
import { useMe } from "@/lib/useMe";

// A lotse.yaml dropped anywhere on the interface, or chosen, is applied as
// lotse apply would apply it: first what it would change, then, on a
// click, the change. What lotse apply would read from where it runs
// ({from_env: NAME}) is asked for instead.

// FileDrop wraps the interface: while a file is dragged over it, it says
// what dropping one does.
export function FileDrop({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const [over, setOver] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const depth = useRef(0);
  const files = (e: React.DragEvent) => e.dataTransfer.types.includes("Files");
  return (
    <div
      className="contents"
      onDragEnter={(e) => {
        if (!isAdmin || !files(e)) return;
        depth.current++;
        setOver(true);
      }}
      onDragLeave={(e) => {
        if (!isAdmin || !files(e)) return;
        if (--depth.current <= 0) {
          depth.current = 0;
          setOver(false);
        }
      }}
      onDragOver={(e) => isAdmin && files(e) && e.preventDefault()}
      onDrop={(e) => {
        if (!isAdmin || !files(e)) return;
        e.preventDefault();
        depth.current = 0;
        setOver(false);
        const f = e.dataTransfer.files[0];
        if (f) setFile(f);
      }}
    >
      {children}
      {over && (
        <div className="pointer-events-none fixed inset-0 z-50 flex items-center justify-center bg-background/70 p-6 backdrop-blur-[2px]">
          <div className="flex max-w-md flex-col items-center gap-3 rounded-xl border-2 border-dashed border-brand/60 bg-card px-10 py-8 text-center shadow-lg">
            <FileUp className="size-8 text-brand" />
            <p className="text-lg font-semibold">{t("applyFile.drop")}</p>
            <p className="text-muted-foreground">{t("applyFile.dropHint")}</p>
          </div>
        </div>
      )}
      <ApplyDialog file={file} onClose={() => setFile(null)} />
    </div>
  );
}

// ChooseFile is a button that applies a lotse.yaml chosen from disk.
export function ChooseFile() {
  const { t } = useTranslation();
  const input = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  return (
    <>
      <Button variant="outline" size="sm" onClick={() => input.current?.click()}>
        <Upload />
        {t("applyFile.choose")}
      </Button>
      <input
        ref={input}
        type="file"
        accept=".yaml,.yml,application/yaml,text/yaml"
        className="hidden"
        onChange={(e) => {
          setFile(e.target.files?.[0] ?? null);
          e.target.value = "";
        }}
      />
      <ApplyDialog file={file} onClose={() => setFile(null)} />
    </>
  );
}

// resolve puts the values given for {from_env: NAME} where they go, as
// lotse apply does with its environment.
function resolve(doc: unknown, values: Record<string, string>): unknown {
  if (Array.isArray(doc)) return doc.map((x) => resolve(x, values));
  if (doc && typeof doc === "object") {
    const o = doc as Record<string, unknown>;
    if (typeof o.from_env === "string" && Object.keys(o).length === 1) return { value: values[o.from_env] ?? "" };
    return Object.fromEntries(Object.entries(o).map(([k, v]) => [k, resolve(v, values)]));
  }
  return doc;
}

function ApplyDialog({ file, onClose }: { file: File | null; onClose: () => void }) {
  return (
    <Dialog open={file !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-2xl">
        {/* A new file starts over. */}
        {file && <ApplyBody key={`${file.name}:${file.lastModified}:${file.size}`} file={file} onClose={onClose} />}
      </DialogContent>
    </Dialog>
  );
}

function ApplyBody({ file, onClose }: { file: File; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [values, setValues] = useState<Record<string, string>>({});
  const doc = (parsed: unknown) => resolve(parsed, values);
  const dry = useMutation({ mutationFn: (parsed: unknown) => applyDocument(doc(parsed), true) });
  const { mutate: previewNow } = dry;
  // A file that needs nothing is previewed at once.
  const parse = useMutation({
    mutationFn: async (f: File) => parseDocument(await f.text()),
    onSuccess: (p) => p.from_env.length === 0 && previewNow(p.document),
  });
  const { mutate: parseFile } = parse;
  useEffect(() => parseFile(file), [file, parseFile]);
  const run = useMutation({
    mutationFn: () => applyDocument(doc(parse.data?.document), false),
    onSuccess: (r) => {
      client.invalidateQueries();
      toast.success(t("applyFile.applied", { count: r.changes.length }));
      onClose();
    },
  });
  const preview = dry.data;
  const needs = parse.data?.from_env ?? [];
  const missing = needs.filter((n) => !values[n]?.trim());
  const error = parse.error ?? dry.error ?? run.error;
  return (
    <>
      <DialogHeader>
        <DialogTitle>{t("applyFile.title", { name: file.name })}</DialogTitle>
        <DialogDescription>{t("applyFile.description")}</DialogDescription>
      </DialogHeader>
      {(parse.isPending || dry.isPending) && <p className="text-muted-foreground">{t("applyFile.reading")}</p>}
      {needs.length > 0 && !preview && (
        <div className="space-y-3">
          <p>{t("applyFile.needs", { count: needs.length })}</p>
          {needs.map((n) => (
            <div key={n} className="space-y-1.5">
              <Label htmlFor={`env-${n}`} className="font-mono">
                {n}
              </Label>
              <Input id={`env-${n}`} type="password" autoComplete="off" value={values[n] ?? ""} onChange={(e) => setValues({ ...values, [n]: e.target.value })} />
            </div>
          ))}
          <p className="text-xs text-muted-foreground">{t("applyFile.needsHint")}</p>
        </div>
      )}
      {preview && <Changes result={preview} />}
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      )}
      <DialogFooter>
        <Button variant="ghost" onClick={onClose}>
          {t("tree.cancel")}
        </Button>
        {preview ? (
          <Button disabled={run.isPending || preview.changes.length === 0} onClick={() => run.mutate()}>
            {t("applyFile.apply", { count: preview.changes.length })}
          </Button>
        ) : (
          needs.length > 0 && (
            <Button disabled={missing.length > 0 || dry.isPending} onClick={() => dry.mutate(parse.data?.document)}>
              {t("applyFile.preview")}
            </Button>
          )
        )}
      </DialogFooter>
    </>
  );
}

const icons = { create: Plus, update: Pencil, delete: Minus };

// short writes a value of a change in a line.
function short(v: unknown): string {
  if (v === undefined || v === null) return "";
  const s = typeof v === "string" ? v : JSON.stringify(v);
  return s.length > 60 ? s.slice(0, 57) + "…" : s;
}

function Changes({ result }: { result: ApplyResult }) {
  const { t } = useTranslation();
  if (result.changes.length === 0) return <p className="rounded-lg border px-4 py-6 text-center text-muted-foreground">{t("applyFile.nothing")}</p>;
  return (
    <div className="space-y-3">
      <ul className="divide-y rounded-lg border">
        {result.changes.map((c: ApplyChange, i) => {
          const Icon = icons[c.action];
          return (
            <li key={i} className="flex items-start gap-3 px-3 py-2.5">
              <Icon className={`mt-0.5 size-4 shrink-0 ${c.action === "delete" ? "text-destructive" : c.action === "create" ? "text-[color:var(--ok)]" : "text-muted-foreground"}`} />
              <div className="min-w-0 flex-1">
                <p className="truncate">
                  <span className="text-muted-foreground">{t(`applyFile.action.${c.action}`, { kind: c.kind })}</span> <span className="font-mono text-mini">{c.path}</span>
                </p>
                {c.field && (
                  <p className="truncate text-mini text-muted-foreground">
                    {c.field}
                    {c.old !== undefined && c.old !== null && <> · {short(c.old)}</>}
                    {c.new !== undefined && c.new !== null && <> → <span className="text-foreground">{short(c.new)}</span></>}
                  </p>
                )}
                {c.last && <p className="text-mini text-muted-foreground">{t("applyFile.last", { by: c.last.by, time: ago(c.last.at), via: c.last.via })}</p>}
              </div>
            </li>
          );
        })}
      </ul>
      {result.warnings.map((w) => (
        <p key={w} className="text-mini text-warn">
          {w}
        </p>
      ))}
    </div>
  );
}
