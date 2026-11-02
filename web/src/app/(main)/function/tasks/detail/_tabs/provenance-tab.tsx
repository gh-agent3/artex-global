"use client";

import * as React from "react";

import { ChevronDownIcon, ChevronRightIcon, DownloadIcon, Trash2Icon } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import type { ProvenanceFile, ProvenanceIntent, TaskProvenance } from "@/lib/types";

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

function formatTime(value?: string): string {
  if (!value) return "—";
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString();
}

// download fetches the file with the bearer token and hands it to the browser as a
// blob, because a plain <a href> would hit the API without the Authorization header.
async function download(taskId: string, path: string) {
  const blob = await api.taskProvenanceFile(taskId, path);
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = path.split("/").pop() ?? "artifact";
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
}

// ProvenanceTab shows what a task's working directory costs on disk, lets the operator
// browse and download individual artifacts, and clears the per-intent scratch dirs.
// Each artifact dir is tied to the graph node that produced it (state + summary), so a
// file can be traced back to the work that wrote it. Every cleanup is audited below.
export function ProvenanceTab({ taskId }: { taskId: string }) {
  const t = useTranslations("taskDetail.provenance");
  const [data, setData] = React.useState<TaskProvenance | null>(null);
  const [error, setError] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [note, setNote] = React.useState("");
  const [version, setVersion] = React.useState(0);
  const [open, setOpen] = React.useState<string>("");
  const [files, setFiles] = React.useState<Record<string, ProvenanceFile[]>>({});
  const [filesError, setFilesError] = React.useState("");

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

  const toggle = async (it: ProvenanceIntent) => {
    if (open === it.name) {
      setOpen("");
      return;
    }
    setOpen(it.name);
    if (files[it.name]) return;
    try {
      const r = await api.taskProvenanceFiles(taskId, it.name);
      setFiles((prev) => ({ ...prev, [it.name]: r.files ?? [] }));
    } catch (e: unknown) {
      setFilesError(e instanceof Error ? e.message : String(e));
    }
  };

  const clean = async () => {
    setBusy(true);
    setError("");
    try {
      await api.taskProvenanceCleanup(taskId, note.trim());
      setNote("");
      setFiles({});
      setOpen("");
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

  const intents = data.intents ?? [];
  const cleanups = data.cleanups ?? [];

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
            <span className="text-muted-foreground">{t("files", { count: data.files })}</span>
          </div>
          <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-muted-foreground">
            <span>
              {t("cleanable")}: {formatBytes(data.cleanable_bytes)}
            </span>
            <span>
              {t("kept")}: {formatBytes(data.other_bytes)}
            </span>
          </div>

          {intents.length > 0 ? (
            <div className="grid gap-1">
              <div className="text-xs font-medium">{t("intentsTitle")}</div>
              <div className="max-h-96 overflow-y-auto rounded-md border">
                <table className="w-full text-xs">
                  <tbody>
                    {intents.slice(0, 80).map((it) => (
                      <React.Fragment key={it.name}>
                        <tr
                          className="cursor-pointer border-b last:border-0 hover:bg-muted/50"
                          onClick={() => void toggle(it)}
                        >
                          <td className="w-6 px-1 py-1">
                            {open === it.name ? (
                              <ChevronDownIcon className="size-3" />
                            ) : (
                              <ChevronRightIcon className="size-3" />
                            )}
                          </td>
                          <td className="px-1 py-1 font-mono">{it.name}</td>
                          <td className="px-1 py-1 text-muted-foreground">
                            {it.state ? `${t("state")}: ${it.state}` : ""}
                          </td>
                          <td className="max-w-72 truncate px-1 py-1 text-muted-foreground" title={it.summary}>
                            {it.summary ?? ""}
                          </td>
                          <td className="px-1 py-1 text-right whitespace-nowrap text-muted-foreground tabular-nums">
                            {formatTime(it.mtime)}
                          </td>
                          <td className="px-1 py-1 text-right tabular-nums">{formatBytes(it.bytes)}</td>
                        </tr>
                        {open === it.name ? (
                          <tr className="border-b bg-muted/20">
                            <td />
                            <td colSpan={5} className="px-1 py-2">
                              {filesError ? (
                                <div className="text-destructive">{filesError}</div>
                              ) : !files[it.name] ? (
                                <div className="text-muted-foreground">{t("loadingFiles")}</div>
                              ) : files[it.name].length === 0 ? (
                                <div className="text-muted-foreground">{t("noFiles")}</div>
                              ) : (
                                <table className="w-full">
                                  <tbody>
                                    {files[it.name].slice(0, 200).map((f) => (
                                      <tr key={f.path} className="border-b last:border-0">
                                        <td className="px-1 py-0.5">
                                          <button
                                            type="button"
                                            className="inline-flex items-center gap-1 font-mono text-left hover:underline"
                                            title={t("download")}
                                            onClick={(e) => {
                                              e.stopPropagation();
                                              void download(taskId, f.path);
                                            }}
                                          >
                                            <DownloadIcon className="size-3 shrink-0" />
                                            {f.path}
                                          </button>
                                        </td>
                                        <td className="px-1 py-0.5 text-right whitespace-nowrap text-muted-foreground tabular-nums">
                                          {formatTime(f.mtime)}
                                        </td>
                                        <td className="px-1 py-0.5 text-right tabular-nums">{formatBytes(f.bytes)}</td>
                                      </tr>
                                    ))}
                                  </tbody>
                                </table>
                              )}
                            </td>
                          </tr>
                        ) : null}
                      </React.Fragment>
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
            <Button
              size="sm"
              variant="destructive"
              disabled={busy || data.cleanable_bytes === 0}
              onClick={() => void clean()}
            >
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
          {cleanups.length === 0 ? (
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
                {cleanups.map((c) => (
                  <tr key={c.id} className="border-b last:border-0">
                    <td className="px-2 py-1 whitespace-nowrap">{formatTime(c.at)}</td>
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
