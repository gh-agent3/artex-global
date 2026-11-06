"use client";

import * as React from "react";

import { LightbulbIcon, Loader2Icon } from "lucide-react";
import { useTranslations } from "next-intl";

import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { getDateLocale } from "@/i18n/datetime";
import { api } from "@/lib/api";
import type { HarnessLesson } from "@/lib/types";

function fmtTime(ts: string) {
  return new Date(ts).toLocaleString(getDateLocale(), {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

const CONFIDENCE_VARIANT: Record<string, "default" | "secondary" | "outline"> = {
  high: "default",
  medium: "secondary",
  low: "outline",
};

export default function LessonsPage() {
  const t = useTranslations("lessonsPage");
  const [lessons, setLessons] = React.useState<HarnessLesson[] | null>(null);

  React.useEffect(() => {
    let alive = true;
    api
      .harnessLessons(200)
      .then((r) => {
        if (alive) setLessons(r.lessons ?? []);
      })
      .catch(() => {
        if (alive) setLessons([]);
      });
    return () => {
      alive = false;
    };
  }, []);

  return (
    <div className="flex flex-col gap-4 p-4 md:p-6">
      <div>
        <h1 className="flex items-center gap-2 text-lg font-semibold">
          <LightbulbIcon className="size-5" />
          {t("title")}
        </h1>
        <p className="text-muted-foreground text-sm">{t("subtitle")}</p>
      </div>

      <Card className="gap-0 py-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-14">ID</TableHead>
              <TableHead className="w-40">{t("col.domain")}</TableHead>
              <TableHead>{t("col.cue")}</TableHead>
              <TableHead>{t("col.lesson")}</TableHead>
              <TableHead className="w-24">{t("col.confidence")}</TableHead>
              <TableHead className="w-20">{t("col.task")}</TableHead>
              <TableHead className="w-28">{t("col.created")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {lessons === null ? (
              <TableRow>
                <TableCell colSpan={7} className="py-10 text-center">
                  <Loader2Icon className="mx-auto size-5 animate-spin" />
                </TableCell>
              </TableRow>
            ) : lessons.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="text-muted-foreground py-10 text-center text-sm">
                  {t("empty")}
                </TableCell>
              </TableRow>
            ) : (
              lessons.map((l) => (
                <TableRow key={l.id}>
                  <TableCell className="font-mono text-xs">{l.id}</TableCell>
                  <TableCell className="text-muted-foreground text-xs">
                    {l.domain || t("global")}
                  </TableCell>
                  <TableCell className="max-w-72 text-sm">{l.cue}</TableCell>
                  <TableCell className="max-w-96 text-sm">{l.lesson}</TableCell>
                  <TableCell>
                    <Badge variant={CONFIDENCE_VARIANT[l.confidence] ?? "outline"}>{l.confidence}</Badge>
                  </TableCell>
                  <TableCell className="font-mono text-xs">{l.task_id ?? "—"}</TableCell>
                  <TableCell className="text-muted-foreground text-xs">{fmtTime(l.created_at)}</TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </Card>
    </div>
  );
}
