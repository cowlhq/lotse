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
      <DialogContent>{open && <Form customer={customer} project={project} done={() => setOpen(false)} />}</DialogContent>
    </Dialog>
  );
}

function Form({ customer: c0, project: p0, done }: { customer: string; project: string; done: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const navigate = useNavigate();
  const scopes = useQuery({ queryKey: ["scopes"], queryFn: listScopes });
  const [customer, setCustomer] = useState(c0);
  const [project, setProject] = useState(p0);
  const [environment, setEnvironment] = useState("");

  const all = scopes.data ?? [];
  // A name the hub already knows keeps its key; a new one gets one.
  const match = (name: string, kind: Scope["kind"], parent?: Scope) =>
    all.find(
      (s) => s.kind === kind && (s.parent_id ?? null) === (parent?.id ?? null) && (s.name === name.trim() || s.slug === slugify(name)),
    );
  const c = match(customer, "customer");
  const p = c && match(project, "project", c);
  const levels = {
    customer: { slug: c?.slug ?? slugify(customer), name: c?.name ?? customer.trim() },
    project: { slug: p?.slug ?? slugify(project), name: p?.name ?? project.trim() },
    environment: { slug: slugify(environment), name: environment.trim() },
  };
  const path = [levels.customer.slug, levels.project.slug, levels.environment.slug].join("/");
  const complete = levels.customer.slug && levels.project.slug && levels.environment.slug;

  const save = useMutation({
    mutationFn: () => createEnvironment(levels),
    onSuccess: async (env) => {
      await client.invalidateQueries({ queryKey: ["scopes"] });
      done();
      navigate({ to: "/environments/$id", params: { id: String(env.id) } });
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
      <DialogHeader>
        <DialogTitle>{t("tree.newEnvironment")}</DialogTitle>
        <DialogDescription>{t("tree.newEnvironmentHint")}</DialogDescription>
      </DialogHeader>
      <Field id="customer" label={t("tree.customer")} value={customer} onChange={setCustomer} options={customers} exists={!!c} />
      <Field id="project" label={t("tree.project")} value={project} onChange={setProject} options={projects} exists={!!p} />
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
