"use client";

import * as React from "react";

import { Trash2Icon } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import type { TaskProvenance } from "@/lib/types";

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value >= 10 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
}

// ProvenanceTab shows what a task's working directory costs on disk and lets the
// operator clear the per-intent scratch dirs (payloads, replicas, logs). The cleanup
// is audited server-side, so the history below survives the deletion.
export function ProvenanceTab({ taskId }: { taskId: string }) {
  const t = useTranslations("taskDetail.provenance");
  const [data, setData] = React.useState<TaskProvenance | null>(null);
  const [error, setError] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [note, setNote] = React.useState("");
  const [version, setVersion] = React.useState(0);

  React.useEffect(() => {
    let disposed = false;
    api
      .taskProvenance(taskId)
      .then((d) => {
        if (!disposed) {
          setData(d);
          setError("");
        }
      })
      .catch((e: unknown) => {
        if (!disposed) setError(e instanceof Error ? e.message : String(e));
      });
    return () => {
      disposed = true;
    };
  }, [taskId, version]);

  const clean = async () => {
    setBusy(true);
    setError("");
    try {
      await api.taskProvenanceCleanup(taskId, note.trim());
      setNote("");
      setVersion((v) => v + 1);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  if (error && !data) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t("title")}</CardTitle>
          <CardDescription>{error}</CardDescription>
        </CardHeader>
      </Card>
    );
  }
  if (!data) {
    return <Skeleton className="h-40 w-full" />;
  }

  return (
    <div className="grid gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{t("title")}</CardTitle>
          <CardDescription className="break-all font-mono text-xs">{data.path}</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 text-sm">
          <div className="flex flex-wrap gap-x-6 gap-y-1">
            <span>
              {t("total")}: <span className="font-medium">{formatBytes(data.bytes)}</span>
            </span>
            <span className="text-muted-foreground">
              {t("files", { count: data.files })}
            </span>
          </div>
          <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-muted-foreground">
            <span>
              {t("cleanable")}: {formatBytes(data.cleanable_bytes)}
            </span>
            <span>
              {t("kept")}: {formatBytes(data.other_bytes)}
            </span>
          </div>

          {(data.intents ?? []).length > 0 ? (
            <div className="grid gap-1">
              <div className="text-xs font-medium">{t("intentsTitle")}</div>
              <div className="max-h-64 overflow-y-auto rounded-md border">
                <table className="w-full text-xs">
                  <tbody>
                    {(data.intents ?? []).slice(0, 60).map((it) => (
                      <tr key={it.name} className="border-b last:border-0">
                        <td className="px-2 py-1 font-mono">{it.name}</td>
                        <td className="px-2 py-1 text-right tabular-nums">{formatBytes(it.bytes)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ) : (
            <div className="text-xs text-muted-foreground">{t("nothingToClean")}</div>
          )}

          <div className="flex flex-wrap items-center gap-2 pt-1">
            <input
              className="h-8 min-w-48 flex-1 rounded-md border bg-transparent px-2 text-xs outline-none focus:border-foreground/40"
              placeholder={t("notePlaceholder")}
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
            <Button size="sm" variant="destructive" disabled={busy || data.cleanable_bytes === 0} onClick={() => void clean()}>
              <Trash2Icon data-icon="inline-start" />
              {busy ? t("cleaning") : t("clean")}
            </Button>
          </div>
          {error ? <div className="text-xs text-destructive">{error}</div> : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">{t("historyTitle")}</CardTitle>
          <CardDescription>{t("historyHint")}</CardDescription>
        </CardHeader>
        <CardContent>
          {(data.cleanups ?? []).length === 0 ? (
            <div className="text-xs text-muted-foreground">{t("historyEmpty")}</div>
          ) : (
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b text-left text-muted-foreground">
                  <th className="px-2 py-1 font-medium">{t("colWhen")}</th>
                  <th className="px-2 py-1 text-right font-medium">{t("colFreed")}</th>
                  <th className="px-2 py-1 text-right font-medium">{t("colFiles")}</th>
                  <th className="px-2 py-1 font-medium">{t("colActor")}</th>
                  <th className="px-2 py-1 font-medium">{t("colNote")}</th>
                </tr>
              </thead>
              <tbody>
                {(data.cleanups ?? []).map((c) => (
                  <tr key={c.id} className="border-b last:border-0">
                    <td className="px-2 py-1 whitespace-nowrap">{new Date(c.at).toLocaleString()}</td>
                    <td className="px-2 py-1 text-right tabular-nums">{formatBytes(c.freed_bytes)}</td>
                    <td className="px-2 py-1 text-right tabular-nums">{c.freed_files}</td>
                    <td className="px-2 py-1">{c.actor}</td>
                    <td className="px-2 py-1 text-muted-foreground">{c.note}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
