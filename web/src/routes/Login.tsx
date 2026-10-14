import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { login } from "@/api";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Centered } from "@/components/shell/Centered";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function Login({ next }: { next: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const navigate = useNavigate();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const submit = useMutation({
    mutationFn: () => login(email, password),
    onSuccess: async () => {
      await client.invalidateQueries();
      // Only a path on this hub, never another site.
      navigate({ to: next.startsWith("/") && !next.startsWith("//") ? next : "/" });
    },
  });

  return (
    <Centered title={t("login.title")} subtitle={t("login.subtitle")}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          submit.mutate();
        }}
        className="space-y-4"
      >
        <div className="space-y-1.5">
          <Label htmlFor="email">{t("login.email")}</Label>
          <Input id="email" type="email" autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} required autoFocus />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="password">{t("login.password")}</Label>
          <Input id="password" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
        </div>
        {submit.isError && (
          <Alert variant="destructive">
            <AlertDescription>{submit.error.message}</AlertDescription>
          </Alert>
        )}
        <Button type="submit" className="w-full" disabled={submit.isPending}>
          {t("login.submit")}
        </Button>
      </form>
    </Centered>
  );
}
