"use client";

import * as React from "react";

import { CheckIcon, LightbulbIcon, Loader2Icon, XIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { getDateLocale } from "@/i18n/datetime";
import { api } from "@/lib/api";
import type { HarnessLesson, HarnessProposal } from "@/lib/types";

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
  const [proposals, setProposals] = React.useState<HarnessProposal[] | null>(null);

  const reload = React.useCallback(() => {
    api
      .harnessLessons(200)
      .then((r) => setLessons(r.lessons ?? []))
      .catch(() => setLessons([]));
    api
      .harnessProposals()
      .then((r) => setProposals(r.proposals ?? []))
      .catch(() => setProposals([]));
  }, []);

  React.useEffect(reload, [reload]);

  const decide = (id: number, apply: boolean) => {
    const call = apply ? api.harnessProposalApply(id) : api.harnessProposalReject(id);
    void call.then(reload).catch(() => reload());
  };

  return (
    <div className="flex flex-col gap-4 p-4 md:p-6">
      <div>
        <h1 className="flex items-center gap-2 text-lg font-semibold">
          <LightbulbIcon className="size-5" />
          {t("title")}
        </h1>
        <p className="text-muted-foreground text-sm">{t("subtitle")}</p>
      </div>

      <Tabs defaultValue="lessons">
        <TabsList>
          <TabsTrigger value="lessons">{t("tab.lessons")}</TabsTrigger>
          <TabsTrigger value="proposals">{t("tab.proposals")}</TabsTrigger>
        </TabsList>

        <TabsContent value="lessons">
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
                      <TableCell className="text-muted-foreground text-xs">{l.domain || t("global")}</TableCell>
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
        </TabsContent>

        <TabsContent value="proposals">
          <Card className="gap-0 py-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-14">ID</TableHead>
                  <TableHead className="w-24">{t("col.agent")}</TableHead>
                  <TableHead>{t("col.summary")}</TableHead>
                  <TableHead className="w-72">{t("col.rationale")}</TableHead>
                  <TableHead className="w-24">{t("col.status")}</TableHead>
                  <TableHead className="w-32">{t("col.actions")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {proposals === null ? (
                  <TableRow>
                    <TableCell colSpan={6} className="py-10 text-center">
                      <Loader2Icon className="mx-auto size-5 animate-spin" />
                    </TableCell>
                  </TableRow>
                ) : proposals.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className="text-muted-foreground py-10 text-center text-sm">
                      {t("proposals.empty")}
                    </TableCell>
                  </TableRow>
                ) : (
                  proposals.map((p) => (
                    <TableRow key={p.id}>
                      <TableCell className="font-mono text-xs">{p.id}</TableCell>
                      <TableCell className="font-mono text-xs">{p.agent_key}</TableCell>
                      <TableCell className="max-w-72 text-sm" title={p.template}>
                        {p.summary}
                      </TableCell>
                      <TableCell className="text-muted-foreground max-w-72 text-xs" title={p.template}>
                        {p.rationale}
                      </TableCell>
                      <TableCell>
                        {p.status === "applied" ? (
                          <Badge variant="default">
                            {t("status.applied")} v{p.prompt_version}
                          </Badge>
                        ) : p.status === "rejected" ? (
                          <Badge variant="outline">{t("status.rejected")}</Badge>
                        ) : (
                          <Badge variant="secondary">{t("status.pending")}</Badge>
                        )}
                      </TableCell>
                      <TableCell>
                        {p.status === "pending" ? (
                          <div className="flex gap-1.5">
                            <Button size="sm" variant="default" onClick={() => decide(p.id, true)}>
                              <CheckIcon className="size-3.5" /> {t("action.apply")}
                            </Button>
                            <Button size="sm" variant="outline" onClick={() => decide(p.id, false)}>
                              <XIcon className="size-3.5" /> {t("action.reject")}
                            </Button>
                          </div>
                        ) : (
                          <span className="text-muted-foreground text-xs">{fmtTime(p.decided_at ?? p.created_at)}</span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
