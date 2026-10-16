import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { acceptLink, checkLink, NotFound } from "@/api";
import { Centered } from "@/components/shell/Centered";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";

// Welcome is where a link from an invitation, or for a new password, leads
// (decision 0023). The token is after the #, so it never reaches a server's
// log.
export function Welcome() {
  const { t } = useTranslation();
  const [token] = useState(() => location.hash.slice(1));
  const link = useQuery({ queryKey: ["link", token], queryFn: () => checkLink(token), enabled: token !== "", retry: false, staleTime: Infinity });
  if (token === "" || link.isError) {
    return (
      <Centered title={t("welcome.deadTitle")}>
        <p className="text-muted-foreground">{token === "" ? t("welcome.noToken") : link.error instanceof NotFound ? t("welcome.dead") : link.error?.message}</p>
      </Centered>
    );
  }
  if (link.isPending) return <Skeleton className="m-6 h-32" />;
  const l = link.data;
  return (
    <Centered title={l.invited ? t("welcome.title") : t("welcome.resetTitle")} subtitle={l.invited ? t("welcome.subtitle") : t("welcome.resetSubtitle")}>
      <Choose token={token} email={l.email} name={l.name} invited={l.invited} />
    </Centered>
  );
}

function Choose({ token, email, name: given, invited }: { token: string; email: string; name: string; invited: boolean }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const navigate = useNavigate();
  const [name, setName] = useState(given);
  const [password, setPassword] = useState("");
  const submit = useMutation({
    mutationFn: () => acceptLink(token, invited ? name.trim() : "", password),
    onSuccess: async () => {
      history.replaceState(null, "", "/welcome");
      await client.invalidateQueries();
      navigate({ to: "/" });
    },
  });
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        submit.mutate();
      }}
    >
      <input type="email" autoComplete="username" value={email} readOnly hidden />
      <div className="space-y-1.5">
        <Label>{t("login.email")}</Label>
        <p className="font-medium">{email}</p>
      </div>
      {invited && (
        <div className="space-y-1.5">
          <Label htmlFor="name">{t("welcome.name")}</Label>
          <Input id="name" autoComplete="name" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </div>
      )}
      <div className="space-y-1.5">
        <Label htmlFor="password">{invited ? t("login.password") : t("welcome.newPassword")}</Label>
        <Input id="password" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} required autoFocus={!invited} />
        <p className="text-xs text-muted-foreground">{t("setup.admin.passwordHint")}</p>
      </div>
      {submit.isError && (
        <Alert variant="destructive">
          <AlertDescription>{submit.error instanceof NotFound ? t("welcome.dead") : submit.error.message}</AlertDescription>
        </Alert>
      )}
      <Button type="submit" className="w-full" disabled={submit.isPending}>
        {invited ? t("welcome.join") : t("welcome.setPassword")}
      </Button>
    </form>
  );
}
