import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Ellipsis, Plus } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { inviteUser, listUsers, newUserLink, updateUser, withdrawInvitation, type User, type UserLink } from "@/api";
import { Command } from "@/components/Command";
import { Panel } from "@/components/shell/Panel";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { ago, dateTime } from "@/lib/format";
import { useMe } from "@/lib/useMe";

// Users are who may log in (decision 0023): admins invite them with a link
// to choose a password with, and make a new one when a password is lost.
export function Users() {
  const { t } = useTranslation();
  const { user: me } = useMe();
  const users = useQuery({ queryKey: ["users"], queryFn: listUsers });
  const [inviting, setInviting] = useState(false);
  const [link, setLink] = useState<UserLink | null>(null);
  const list = users.data ?? [];
  return (
    <>
      <Panel
        title={t("user.title")}
        count={list.filter((u) => u.state !== "disabled").length}
        right={
          <Button size="sm" variant="outline" onClick={() => setInviting(true)}>
            <Plus />
            {t("user.invite")}
          </Button>
        }
        flush
      >
        <p className="border-b px-4 py-3 text-muted-foreground">{t("user.description")}</p>
        <ul>
          {list.map((u) => (
            <li key={u.id} className={`flex items-center gap-3 border-b px-4 py-3 last:border-b-0 ${u.state === "disabled" ? "opacity-60" : ""}`}>
              <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted text-ui font-semibold">{(u.name || u.email).slice(0, 1).toUpperCase()}</span>
              <div className="min-w-0 flex-1">
                <p className="flex flex-wrap items-center gap-x-2 font-medium">
                  <span className="truncate">{u.name || u.email}</span>
                  {u.id === me?.id && <span className="text-mini font-normal text-muted-foreground">{t("user.you")}</span>}
                </p>
                <p className="truncate text-mini text-muted-foreground">
                  {u.name && <>{u.email} · </>}
                  <State user={u} />
                </p>
              </div>
              <Badge variant={u.role === "admin" ? "secondary" : "outline"}>{t(`user.role.${u.role}`)}</Badge>
              {u.id === me?.id ? <span className="w-8" /> : <Actions user={u} onLink={setLink} />}
            </li>
          ))}
        </ul>
      </Panel>
      <Dialog open={inviting} onOpenChange={setInviting}>
        <DialogContent className="sm:max-w-lg">
          {inviting && (
            <InviteForm
              onDone={(x) => {
                setInviting(false);
                setLink(x);
              }}
            />
          )}
        </DialogContent>
      </Dialog>
      <Dialog open={link !== null} onOpenChange={(o) => !o && setLink(null)}>
        <DialogContent className="sm:max-w-lg">{link && <Link link={link} />}</DialogContent>
      </Dialog>
    </>
  );
}

function State({ user: u }: { user: User }) {
  const { t } = useTranslation();
  const [now] = useState(() => Date.now() / 1000);
  if (u.state === "disabled") return <>{t("user.state.disabled")}</>;
  if (u.state === "invited") {
    const expired = !u.link_expires_at || u.link_expires_at < now;
    return <>{expired ? t("user.state.invitedExpired") : t("user.state.invited", { time: dateTime(u.link_expires_at!) })}</>;
  }
  return <>{u.last_login_at ? t("user.state.lastLogin", { time: ago(u.last_login_at) }) : t("user.state.never")}</>;
}

function Actions({ user: u, onLink }: { user: User; onLink: (x: UserLink) => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const done = () => client.invalidateQueries({ queryKey: ["users"] });
  const fail = (e: Error) => toast.error(e.message);
  const change = useMutation({ mutationFn: (c: Parameters<typeof updateUser>[1]) => updateUser(u.id, c), onSuccess: done, onError: fail });
  const link = useMutation({ mutationFn: () => newUserLink(u.id), onSuccess: (x) => (done(), onLink(x)), onError: fail });
  const withdraw = useMutation({ mutationFn: () => withdrawInvitation(u.id), onSuccess: () => (done(), toast.success(t("user.withdrawn"))), onError: fail });
  const who = u.name || u.email;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button variant="ghost" size="icon-sm" aria-label={t("user.actions")} />}>
        <Ellipsis />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-60">
        {u.state !== "disabled" && (
          <DropdownMenuItem onClick={() => link.mutate()}>{u.state === "invited" ? t("user.newInvitation") : t("user.newPassword")}</DropdownMenuItem>
        )}
        <DropdownMenuItem onClick={() => change.mutate({ role: u.role === "admin" ? "viewer" : "admin" })}>
          {u.role === "admin" ? t("user.makeViewer") : t("user.makeAdmin")}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        {u.state === "invited" ? (
          <DropdownMenuItem variant="destructive" onClick={() => confirm(t("user.withdrawConfirm", { who })) && withdraw.mutate()}>
            {t("user.withdraw")}
          </DropdownMenuItem>
        ) : u.state === "disabled" ? (
          <DropdownMenuItem onClick={() => change.mutate({ disabled: false })}>{t("user.enable")}</DropdownMenuItem>
        ) : (
          <DropdownMenuItem variant="destructive" onClick={() => confirm(t("user.disableConfirm", { who })) && change.mutate({ disabled: true })}>
            {t("user.disable")}
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function InviteForm({ onDone }: { onDone: (x: UserLink) => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [role, setRole] = useState<User["role"]>("viewer");
  const save = useMutation({
    mutationFn: () => inviteUser(email.trim(), name.trim(), role),
    onSuccess: (x) => (client.invalidateQueries({ queryKey: ["users"] }), onDone(x)),
  });
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <DialogHeader>
        <DialogTitle>{t("user.invite")}</DialogTitle>
        <DialogDescription>{t("user.inviteHint")}</DialogDescription>
      </DialogHeader>
      <div className="space-y-1.5">
        <Label htmlFor="invite-email">{t("login.email")}</Label>
        <Input id="invite-email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="name@example.com" required autoFocus />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="invite-name">{t("user.name")}</Label>
        <Input id="invite-name" value={name} onChange={(e) => setName(e.target.value)} placeholder={t("user.namePlaceholder")} />
      </div>
      <div className="space-y-1.5">
        <Label>{t("user.roleLabel")}</Label>
        <ToggleGroup value={[role]} onValueChange={(v: string[]) => v[0] && setRole(v[0] as User["role"])} variant="segmented" size="sm">
          <ToggleGroupItem value="viewer">{t("user.role.viewer")}</ToggleGroupItem>
          <ToggleGroupItem value="admin">{t("user.role.admin")}</ToggleGroupItem>
        </ToggleGroup>
        <p className="text-mini text-muted-foreground">{t(`user.roleHint.${role}`)}</p>
      </div>
      {save.isError && (
        <Alert variant="destructive">
          <AlertDescription>{save.error.message}</AlertDescription>
        </Alert>
      )}
      <div className="flex justify-end">
        <Button type="submit" disabled={save.isPending || !email.trim()}>
          {save.isPending ? t("user.sending") : t("user.inviteSubmit")}
        </Button>
      </div>
    </form>
  );
}

// Link shows a new link once, and whether the hub sent it.
function Link({ link }: { link: UserLink }) {
  const { t } = useTranslation();
  const u = link.user;
  const invited = u.state === "invited";
  const hint = link.mailed
    ? t("user.linkMailed", { email: u.email })
    : link.mail_error
      ? t("user.linkMailFailed", { error: link.mail_error })
      : t("user.linkNoMail");
  return (
    <div className="space-y-4">
      <DialogHeader>
        <DialogTitle>{invited ? t("user.linkTitle", { who: u.name || u.email }) : t("user.resetTitle", { who: u.name || u.email })}</DialogTitle>
        <DialogDescription>{hint}</DialogDescription>
      </DialogHeader>
      <Command text={link.link} hint={t("user.linkHint", { time: dateTime(u.link_expires_at!) })} />
    </div>
  );
}
