import { useQuery } from "@tanstack/react-query";
import { Bell, BellRing } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { getConfig } from "@/api";
import { Rule } from "@/components/Rules";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { useMe } from "@/lib/useMe";

// AlertBell sets the alerts of one host or check, as Beszel does (see
// Rule). What is set here applies to this object only; what is not set
// comes from above.

type Kind = "host" | "check" | "heartbeat";

const rulesOf: Record<Kind, string[]> = {
  host: ["host.silent", "host.cpu", "host.memory", "host.disk", "host.swap", "host.load", "host.iowait", "host.unit", "host.service", "host.reboot", "container.unhealthy", "container.restarting"],
  check: ["check.down", "check.cert"],
  heartbeat: ["check.heartbeat"],
};

export function AlertBell({ subject, name, kind }: { subject: string; name: string; kind: Kind }) {
  const { t } = useTranslation();
  const { isAdmin } = useMe();
  const [open, setOpen] = useState(false);
  const config = useQuery({ queryKey: ["config", subject], queryFn: () => getConfig(subject), enabled: open });
  // A bell that rings says something is set here.
  const own = (config.data ?? []).some((v) => v.key.startsWith("rules.") && v.from && `${v.from.kind}:${v.from.id}` === subject);
  if (!isAdmin) return null;
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button variant="ghost" size="icon-sm" aria-label={t("bell.title", { name })} />}>
        {own ? <BellRing className="text-brand" /> : <Bell />}
      </DialogTrigger>
      <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("bell.title", { name })}</DialogTitle>
          <DialogDescription>{t("bell.description")}</DialogDescription>
        </DialogHeader>
        {config.data && (
          <ul className="-mx-5 divide-y border-y">
            {rulesOf[kind].map((rule) => (
              <Rule key={rule} subject={subject} rule={rule} values={config.data} />
            ))}
          </ul>
        )}
      </DialogContent>
    </Dialog>
  );
}
