import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Wrench } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { createMaintenance, deleteMaintenance, listMaintenance, type Maintenance as Window } from "@/api";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { dateTime } from "@/lib/format";
import { useMe } from "@/lib/useMe";

// Maintenance lists the windows that cover an object or scope, its own and
// those of scopes above it, and adds windows to it: once, or by schedule.
export function Maintenance({ subject }: { subject: string }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const client = useQueryClient();
  const [adding, setAdding] = useState(false);
  const windows = useQuery({ queryKey: ["maintenance", subject], queryFn: () => listMaintenance(subject), refetchInterval: 60_000 });
  const remove = useMutation({
    mutationFn: deleteMaintenance,
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["maintenance"] });
      client.invalidateQueries({ queryKey: ["alerts"] });
      client.invalidateQueries({ queryKey: ["changes"] });
    },
  });
  const [now] = useState(() => Date.now() / 1000);
  // Windows that are over stay in the history; here, what is now or to come.
  const list = (windows.data ?? []).filter((w) => w.schedule || (w.ends_at ?? 0) > now);
  const own = (w: Window) => `${w.subject_kind}:${w.subject_id}` === subject;
  if (!isAdmin && list.length === 0) return null;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm font-medium">{t("maintenance.title")}</CardTitle>
        <CardDescription>{t("maintenance.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        {list.length === 0 && <p className="text-muted-foreground">{t("maintenance.none")}</p>}
        <ul className="divide-y">
          {list.map((w) => (
            <li key={w.id} className="flex items-center justify-between gap-4 py-2">
              <span className="min-w-0">
                {w.active && (
                  <span className="mr-2 inline-flex items-center gap-1 font-medium">
                    <Wrench className="size-3.5" />
                    {t("maintenance.now")}
                  </span>
                )}
                {w.schedule ? (
                  <code>{w.schedule}</code>
                ) : (
                  <span className="tabular-nums">
                    {dateTime(w.starts_at!)} – {dateTime(w.ends_at!)}
                  </span>
                )}
                {w.reason && <span className="ml-2">{w.reason}</span>}
                <span className="ml-2 text-muted-foreground">
                  {!own(w) && <>{t("maintenance.on", { path: w.subject_path })} · </>}
                  {w.created_by}
                </span>
              </span>
              {isAdmin && own(w) && (
                <Button variant="ghost" size="sm" disabled={remove.isPending} onClick={() => remove.mutate(w.id)}>
                  {w.active && !w.schedule ? t("maintenance.end") : t("maintenance.remove")}
                </Button>
              )}
            </li>
          ))}
        </ul>
        {isAdmin &&
          (adding ? (
            <Add subject={subject} onDone={() => setAdding(false)} />
          ) : (
            <Button size="sm" variant="outline" onClick={() => setAdding(true)}>
              <Plus className="size-4" />
              {t("maintenance.add")}
            </Button>
          ))}
      </CardContent>
    </Card>
  );
}

// local writes a time as a datetime-local input wants it.
function local(ts: number): string {
  const d = new Date(ts * 1000);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function Add({ subject, onDone }: { subject: string; onDone: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [kind, setKind] = useState<"once" | "schedule">("once");
  const [start, setStart] = useState(() => local(Math.floor(Date.now() / 60_000) * 60));
  const [end, setEnd] = useState(() => local(Math.floor(Date.now() / 60_000) * 60 + 3600));
  const [schedule, setSchedule] = useState("");
  const [reason, setReason] = useState("");
  const save = useMutation({
    mutationFn: () =>
      createMaintenance(
        kind === "once"
          ? { subject, starts_at: new Date(start).getTime() / 1000, ends_at: new Date(end).getTime() / 1000, reason: reason.trim() }
          : { subject, schedule: schedule.trim(), reason: reason.trim() },
      ),
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["maintenance"] });
      client.invalidateQueries({ queryKey: ["alerts"] });
      client.invalidateQueries({ queryKey: ["changes"] });
      onDone();
    },
  });
  return (
    <form
      className="max-w-lg space-y-3 rounded-lg border p-3"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <ToggleGroup value={[kind]} onValueChange={(v: string[]) => v[0] && setKind(v[0] as typeof kind)} variant="outline" size="sm">
        <ToggleGroupItem value="once">{t("maintenance.once")}</ToggleGroupItem>
        <ToggleGroupItem value="schedule">{t("maintenance.schedule")}</ToggleGroupItem>
      </ToggleGroup>
      {kind === "once" ? (
        <div className="grid grid-cols-2 gap-3">
          <div className="space-y-1.5">
            <Label htmlFor="mw-start">{t("maintenance.from")}</Label>
            <Input id="mw-start" type="datetime-local" value={start} onChange={(e) => setStart(e.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="mw-end">{t("maintenance.until")}</Label>
            <Input id="mw-end" type="datetime-local" value={end} onChange={(e) => setEnd(e.target.value)} />
          </div>
        </div>
      ) : (
        <div className="space-y-1.5">
          <Label htmlFor="mw-schedule">{t("maintenance.schedule")}</Label>
          <Input id="mw-schedule" value={schedule} onChange={(e) => setSchedule(e.target.value)} placeholder="sun 02:00-04:00" autoComplete="off" />
          <p className="text-xs text-muted-foreground">{t("maintenance.scheduleHint")}</p>
        </div>
      )}
      <div className="space-y-1.5">
        <Label htmlFor="mw-reason">{t("maintenance.reason")}</Label>
        <Input id="mw-reason" value={reason} onChange={(e) => setReason(e.target.value)} placeholder={t("maintenance.reasonPlaceholder")} />
      </div>
      {save.isError && (
        <Alert variant="destructive">
          <AlertDescription>{save.error.message}</AlertDescription>
        </Alert>
      )}
      <div className="flex gap-2">
        <Button type="submit" size="sm" disabled={save.isPending || (kind === "schedule" && !schedule.trim())}>
          {t("maintenance.save")}
        </Button>
        <Button type="button" size="sm" variant="ghost" onClick={onDone}>
          {t("tree.cancel")}
        </Button>
      </div>
    </form>
  );
}
