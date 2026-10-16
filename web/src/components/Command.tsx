import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";

// Command shows a command to copy, and what it does.
export function Command({ text, hint }: { text: string; hint: string }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  return (
    <div className="space-y-2 pt-2">
      <div className="relative">
        <pre className="max-h-80 overflow-auto rounded-lg border bg-muted/60 p-3 pr-12 font-mono text-mini leading-relaxed whitespace-pre-wrap [overflow-wrap:anywhere]">
          {text}
        </pre>
        <Button
          variant="ghost"
          size="icon-sm"
          className="absolute top-2 right-2"
          aria-label={t("agent.copy")}
          onClick={() => {
            navigator.clipboard?.writeText(text);
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
          }}
        >
          {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
        </Button>
      </div>
      <p className="text-mini text-muted-foreground">{hint}</p>
    </div>
  );
}
