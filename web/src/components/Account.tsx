import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { updateMe, type User } from "@/api";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

// Account is one's own name and password. A new password ends every other
// session (decision 0023).
export function Account({ me, open, onOpenChange }: { me: User; open: boolean; onOpenChange: (open: boolean) => void }) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">{open && <AccountForm me={me} onDone={() => onOpenChange(false)} />}</DialogContent>
    </Dialog>
  );
}

function AccountForm({ me, onDone }: { me: User; onDone: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [name, setName] = useState(me.name);
  const [current, setCurrent] = useState("");
  const [password, setPassword] = useState("");
  const save = useMutation({
    mutationFn: () =>
      updateMe({
        ...(name.trim() !== me.name ? { name: name.trim() } : {}),
        ...(password ? { password, current_password: current } : {}),
      }),
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["me"] });
      client.invalidateQueries({ queryKey: ["users"] });
      toast.success(password ? t("account.passwordChanged") : t("account.saved"));
      onDone();
    },
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
        <DialogTitle>{t("account.title")}</DialogTitle>
        <DialogDescription>{me.email}</DialogDescription>
      </DialogHeader>
      <input type="email" autoComplete="username" value={me.email} readOnly hidden />
      <div className="space-y-1.5">
        <Label htmlFor="account-name">{t("user.name")}</Label>
        <Input id="account-name" autoComplete="name" value={name} onChange={(e) => setName(e.target.value)} />
      </div>
      <div className="space-y-3 border-t pt-4">
        <p className="font-medium">{t("account.changePassword")}</p>
        <div className="space-y-1.5">
          <Label htmlFor="account-current">{t("account.current")}</Label>
          <Input id="account-current" type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} required={password !== ""} />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="account-new">{t("welcome.newPassword")}</Label>
          <Input id="account-new" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} />
          <p className="text-xs text-muted-foreground">{t("account.passwordHint")}</p>
        </div>
      </div>
      {save.isError && (
        <Alert variant="destructive">
          <AlertDescription>{save.error.message}</AlertDescription>
        </Alert>
      )}
      <div className="flex justify-end">
        <Button type="submit" disabled={save.isPending || (name.trim() === me.name && !password)}>
          {t("account.save")}
        </Button>
      </div>
    </form>
  );
}
