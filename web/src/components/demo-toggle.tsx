"use client";

import { EyeIcon, EyeOffIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

import { useDemoMode } from "./demo-mode";

// DemoToggle is the eye in the header. While demo mode is on it stays visibly pressed, so
// nobody records a walkthrough without noticing.
export function DemoToggle({ className }: { className?: string }) {
  const { demo, toggle } = useDemoMode();
  const t = useTranslations("demo");

  return (
    <Button
      type="button"
      size="icon"
      variant={demo ? "default" : "ghost"}
      className={cn("relative size-8", className)}
      aria-pressed={demo}
      title={demo ? t("turnOff") : t("turnOn")}
      onClick={toggle}
    >
      {demo ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
      <span className="sr-only">{demo ? t("turnOff") : t("turnOn")}</span>
    </Button>
  );
}

// DemoBadge is the small "Demo mode" pill shown next to the title while masking is active.
export function DemoBadge() {
  const { demo } = useDemoMode();
  const t = useTranslations("demo");
  if (!demo) return null;
  return (
    <span className="rounded-full border border-amber-400/60 bg-amber-400/10 px-2 py-0.5 text-[10px] font-medium text-amber-700 dark:text-amber-400">
      {t("badge")}
    </span>
  );
}
