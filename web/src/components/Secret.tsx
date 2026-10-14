import { useTranslation } from "react-i18next";
import type { SecretInfo, SecretInput } from "@/api";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

// A secret is typed in, and then kept sealed by the hub, or read by the
// hub when it sends: from an environment variable, or a file in
// /run/secrets. The hub never shows it again; it only says that it is set.

export type SecretDraft = { mode: "value" | "env" | "file"; text: string };

export const emptySecret: SecretDraft = { mode: "value", text: "" };

// secretInput is what to send for a draft: nothing when nothing was typed,
// which keeps what is set.
export function secretInput(d: SecretDraft): SecretInput | undefined {
  const text = d.text.trim();
  if (!text) return undefined;
  return { [d.mode]: text };
}

export function SecretField({
  id,
  label,
  hint,
  info,
  draft,
  onChange,
  placeholder,
}: {
  id: string;
  label: string;
  hint?: string;
  info?: SecretInfo;
  draft: SecretDraft;
  onChange: (d: SecretDraft) => void;
  placeholder?: string;
}) {
  const { t } = useTranslation();
  let set = "";
  if (info?.set) set = info.env ? t("secret.fromEnv", { name: info.env }) : info.file ? t("secret.fromFile", { name: info.file }) : t("secret.sealed");
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between gap-2">
        <Label htmlFor={id}>{label}</Label>
        <ToggleGroup value={[draft.mode]} onValueChange={(v: string[]) => v[0] && onChange({ mode: v[0] as SecretDraft["mode"], text: "" })} variant="outline" size="sm">
          {(["value", "env", "file"] as const).map((m) => (
            <ToggleGroupItem key={m} value={m} className="text-xs">
              {t(`secret.mode.${m}`)}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      </div>
      <Input
        id={id}
        type={draft.mode === "value" ? "password" : "text"}
        autoComplete="off"
        value={draft.text}
        onChange={(e) => onChange({ ...draft, text: e.target.value })}
        placeholder={draft.mode === "env" ? "ACME_CHAT_WEBHOOK" : draft.mode === "file" ? "/run/secrets/chat-webhook" : set ? t("secret.keep") : placeholder}
      />
      <p className="text-xs text-muted-foreground">
        {set && <>{t("secret.isSet", { how: set })} </>}
        {hint && <>{hint} </>}
        {t(`secret.hint.${draft.mode}`)}
      </p>
    </div>
  );
}
