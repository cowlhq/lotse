import { toast } from "sonner";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Plus } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { createEnvironment, listScopes, type Scope } from "@/api";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useMe } from "@/lib/useMe";
import { slugify } from "@/lib/slug";

// NewEnvironment asks for three names: customer, project, environment.
// Existing customers and projects are offered as they are typed; keys come
// from the names.
export function NewEnvironment({ customer = "", project = "" }: { customer?: string; project?: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm" />}>
        <Plus className="size-4" />
        {t("tree.newEnvironment")}
      </DialogTrigger>
      <DialogContent>{open && <EnvironmentForm customer={customer} project={project} done={() => setOpen(false)} />}</DialogContent>
    </Dialog>
  );
}

export function EnvironmentForm({ customer: c0, project: p0, done, onCreated, bare = false }: { customer: string; project: string; done: () => void; onCreated?: (env: Scope) => void; bare?: boolean }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const navigate = useNavigate();
  const scopes = useQuery({ queryKey: ["scopes"], queryFn: listScopes });
  const { user, scoped } = useMe();
  const [customerText, setCustomer] = useState(c0);
  const [projectText, setProject] = useState(p0);
  // Most start with production; it is only a suggestion.
  const [environment, setEnvironment] = useState("Production");

  const all = scopes.data ?? [];
  // A user kept to a customer or project adds environments only there, so
  // those are given; by name if the user sees it, by key if not.
  const fixed = scoped ? (user?.scope_path ?? "").split("/") : [];
  const given = (n: number) => {
    const path = fixed.slice(0, n).join("/");
    return all.find((s) => s.path === path)?.name ?? fixed[n - 1];
  };
  const customer = fixed.length >= 1 ? given(1) : customerText;
  const project = fixed.length >= 2 ? given(2) : projectText;
  // A name the hub already knows keeps its key; a new one gets one.
  const match = (name: string, kind: Scope["kind"], parent?: Scope) =>
    all.find(
      (s) => s.kind === kind && (s.parent_id ?? null) === (parent?.id ?? null) && (s.name === name.trim() || s.slug === slugify(name)),
    );
  const c = match(customer, "customer");
  const p = c && match(project, "project", c);
  const levels = {
    customer: { slug: fixed[0] ?? c?.slug ?? slugify(customer), name: c?.name ?? customer.trim() },
    project: { slug: fixed[1] ?? p?.slug ?? slugify(project), name: p?.name ?? project.trim() },
    environment: { slug: slugify(environment), name: environment.trim() },
  };
  const path = [levels.customer.slug, levels.project.slug, levels.environment.slug].join("/");
  const complete = levels.customer.slug && levels.project.slug && levels.environment.slug;

  const save = useMutation({
    mutationFn: () => createEnvironment(levels),
    onSuccess: async (env) => {
      await client.invalidateQueries({ queryKey: ["scopes"] });
      done();
      toast.success(t("tree.created", { name: env.name }));
      if (onCreated) onCreated(env);
      else navigate({ to: "/scopes/$id", params: { id: String(env.id) } });
    },
  });

  const customers = all.filter((s) => s.kind === "customer");
  const projects = c ? all.filter((s) => s.kind === "project" && s.parent_id === c.id) : [];

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
      className="space-y-4"
    >
      <DialogHeader className={bare ? "hidden" : undefined}>
        <DialogTitle>{t("tree.newEnvironment")}</DialogTitle>
        <DialogDescription>{t("tree.newEnvironmentHint")}</DialogDescription>
      </DialogHeader>
      <Field id="customer" label={t("tree.customer")} value={customer} onChange={setCustomer} options={customers} exists={!!c || fixed.length >= 1} disabled={fixed.length >= 1} />
      <Field id="project" label={t("tree.project")} value={project} onChange={setProject} options={projects} exists={!!p || fixed.length >= 2} disabled={fixed.length >= 2} />
      <Field id="environment" label={t("tree.environment")} value={environment} onChange={setEnvironment} placeholder="production" />
      {complete && (
        <p className="text-xs text-muted-foreground">
          {t("tree.key")} <code>{path}</code>
        </p>
      )}
      {save.isError && (
        <Alert variant="destructive">
          <AlertDescription>{save.error.message}</AlertDescription>
        </Alert>
      )}
      <DialogFooter>
        <Button type="submit" disabled={!complete || save.isPending}>
          {t("tree.create")}
        </Button>
      </DialogFooter>
    </form>
  );
}

function Field(props: {
  id: string;
  label: string;
  value: string;
  onChange: (v: string) => void;
  options?: Scope[];
  exists?: boolean;
  placeholder?: string;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <Label htmlFor={props.id} className="flex justify-between">
        {props.label}
        {props.value && props.options && (
          <span className="text-xs font-normal text-muted-foreground">{props.exists ? t("tree.exists") : t("tree.new")}</span>
        )}
      </Label>
      <Input
        id={props.id}
        list={props.options ? `${props.id}-options` : undefined}
        value={props.value}
        placeholder={props.placeholder}
        onChange={(e) => props.onChange(e.target.value)}
        autoComplete="off"
        disabled={props.disabled}
        required
      />
      {props.options && (
        <datalist id={`${props.id}-options`}>
          {props.options.map((o) => (
            <option key={o.id} value={o.name} />
          ))}
        </datalist>
      )}
    </div>
  );
}
