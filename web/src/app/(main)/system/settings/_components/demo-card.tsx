"use client";

import * as React from "react";

import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { api } from "@/lib/api";
import { DEMO_CLASSES, type DemoClass } from "@/lib/demo";

// DemoCard configures WHAT demo mode masks: the sensitive classes and the operator's own
// masking rules (regular expressions applied to free text such as evidence blocks).
// The on/off switch itself stays on the eye in the header.
export function DemoCard() {
  const t = useTranslations("demo");
  const [classes, setClasses] = React.useState<DemoClass[]>([...DEMO_CLASSES]);
  const [rules, setRules] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [sample, setSample] = React.useState("");

  React.useEffect(() => {
    api
      .demoSettings()
      .then((s) => {
        setClasses((s.classes ?? []).filter((c): c is DemoClass => (DEMO_CLASSES as readonly string[]).includes(c)));
        setRules((s.rules ?? []).join("\n"));
      })
      .catch(() => {
        /* keep the defaults */
      });
  }, []);

  const toggleClass = (c: DemoClass) => {
    setClasses((prev) => (prev.includes(c) ? prev.filter((x) => x !== c) : [...prev, c]));
  };

  const save = async () => {
    setBusy(true);
    try {
      await api.demoSettingsSave({
        classes,
        rules: rules
          .split("\n")
          .map((r) => r.trim())
          .filter(Boolean),
      });
      toast.success(t("saved"));
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  // Live preview: apply the current rules to a sample line.
  const preview = React.useMemo(() => {
    if (!sample) return "";
    let out = sample;
    for (const raw of rules.split("\n").map((r) => r.trim()).filter(Boolean)) {
      try {
        out = out.replace(new RegExp(raw, "gi"), "•••");
      } catch {
        /* a broken pattern is reported on save */
      }
    }
    return out;
  }, [sample, rules]);

  return (
    <Card className="mb-4 break-inside-avoid md:mb-6">
      <CardHeader>
        <CardTitle>{t("settingsTitle")}</CardTitle>
        <CardDescription>{t("settingsHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <div className="grid gap-2">
          <div className="text-xs font-medium">{t("classesTitle")}</div>
          <div className="flex flex-wrap gap-x-4 gap-y-2">
            {DEMO_CLASSES.map((c) => (
              <label key={c} className="flex items-center gap-2 text-xs">
                <input
                  type="checkbox"
                  className="size-3.5"
                  checked={classes.includes(c)}
                  onChange={() => toggleClass(c)}
                />
                <span>{t(`class.${c}`)}</span>
              </label>
            ))}
          </div>
        </div>

        <div className="grid gap-2">
          <div className="text-xs font-medium">{t("rulesTitle")}</div>
          <div className="text-xs text-muted-foreground">{t("rulesHint")}</div>
          <textarea
            className="min-h-28 w-full rounded-md border bg-transparent p-2 font-mono text-xs outline-none focus:border-foreground/40"
            placeholder={t("rulesPlaceholder")}
            value={rules}
            onChange={(e) => setRules(e.target.value)}
          />
        </div>

        <div className="grid gap-2">
          <div className="text-xs font-medium">{t("previewTitle")}</div>
          <input
            className="h-8 rounded-md border bg-transparent px-2 text-xs outline-none focus:border-foreground/40"
            placeholder={t("previewPlaceholder")}
            value={sample}
            onChange={(e) => setSample(e.target.value)}
          />
          {sample ? <code className="break-all rounded-md bg-muted p-2 text-xs">{preview}</code> : null}
        </div>

        <div>
          <Button size="sm" onClick={() => void save()} disabled={busy}>
            {busy ? t("saving") : t("save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
