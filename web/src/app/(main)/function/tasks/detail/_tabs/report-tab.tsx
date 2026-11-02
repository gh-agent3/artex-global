"use client";

import * as React from "react";

import { CheckIcon, CopyIcon, DownloadIcon, FileTextIcon, PencilIcon, SendIcon } from "lucide-react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { DemoMarkdown } from "@/components/demo-markdown";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { api } from "@/lib/api";
import type { ReportProfile, ReportVersion } from "@/lib/types";
import { copyText } from "@/lib/utils";

export function ReportTab({ taskId }: { taskId: string }) {
  // A different task must never render or copy the previous task's report.
  return <TaskReport key={taskId} taskId={taskId} />;
}

// TaskReport shows the stored report versions as a tree, lets the operator download any
// of them, pick the report format (profile), edit that format in place, and ask the
// report writer agent for a revision of the selected version.
function TaskReport({ taskId }: { taskId: string }) {
  const t = useTranslations("taskDetail.report");
  const [versions, setVersions] = React.useState<ReportVersion[] | null>(null);
  const [selected, setSelected] = React.useState<ReportVersion | null>(null);
  const [profiles, setProfiles] = React.useState<ReportProfile[]>([]);
  const [profileID, setProfileID] = React.useState(0);
  const [editing, setEditing] = React.useState(false);
  const [formatDraft, setFormatDraft] = React.useState("");
  const [formatName, setFormatName] = React.useState("");
  const [instruction, setInstruction] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState("");
  const [copied, setCopied] = React.useState(false);

  const load = React.useCallback(async () => {
    try {
      const [list, settings, profs] = await Promise.all([
        api.taskReportVersions(taskId),
        api.taskReportSettings(taskId),
        api.reportProfiles(),
      ]);
      setVersions(list);
      setProfiles(profs);
      setProfileID(settings.profile_id ?? 0);
      const first = list[0];
      if (first) {
        const full = await api.taskReportVersion(taskId, first.id);
        setSelected(full);
      }
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
      setVersions([]);
    }
  }, [taskId]);

  React.useEffect(() => {
    void load();
  }, [load]);

  const openVersion = async (v: ReportVersion) => {
    try {
      setSelected(await api.taskReportVersion(taskId, v.id));
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const download = async () => {
    if (!selected) return;
    try {
      const blob = await api.taskReportDownload(taskId, selected.id);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `report-${taskId}-${selected.version}.md`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(url), 10_000);
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  };

  const copy = async () => {
    if (!selected?.content) return;
    const ok = await copyText(selected.content);
    if (ok) {
      setCopied(true);
      toast.success(t("copySuccess"));
      setTimeout(() => setCopied(false), 1500);
    } else {
      toast.error(t("copyError"));
    }
  };

  const startEditFormat = () => {
    const p = profiles.find((x) => x.id === profileID) ?? profiles.find((x) => x.builtin) ?? profiles[0];
    setFormatName(p?.name ?? "");
    setFormatDraft(p?.instructions ?? "");
    setEditing(true);
  };

  const saveFormat = async () => {
    const p = profiles.find((x) => x.id === profileID) ?? profiles.find((x) => x.builtin) ?? profiles[0];
    setBusy(true);
    try {
      const r = await api.reportProfileSave({ id: p?.id, name: formatName.trim() || p?.name || "Custom format", instructions: formatDraft });
      await api.taskReportSettingsSave(taskId, r.id);
      setProfileID(r.id);
      setEditing(false);
      const profs = await api.reportProfiles();
      setProfiles(profs);
      toast.success(t("formatSaved"));
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const selectProfile = async (id: number) => {
    setProfileID(id);
    try {
      await api.taskReportSettingsSave(taskId, id);
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  };

  const revise = async () => {
    if (!selected || !instruction.trim()) return;
    setBusy(true);
    setError("");
    try {
      const v = await api.taskReportRevise(taskId, selected.id, instruction.trim());
      setInstruction("");
      setVersions(await api.taskReportVersions(taskId));
      setSelected(v);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  if (versions === null) return <Skeleton className="h-64 w-full" />;

  return (
    <div className="grid gap-4 lg:grid-cols-[220px_1fr]">
      <Card className="h-fit">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">{t("versionsTitle")}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-1">
          {versions.length === 0 ? (
            <div className="text-xs text-muted-foreground">{t("versionsEmpty")}</div>
          ) : (
            versions.map((v) => (
              <button
                key={v.id}
                type="button"
                onClick={() => void openVersion(v)}
                className={`rounded-md px-2 py-1 text-left text-xs transition-colors hover:bg-muted ${
                  selected?.id === v.id ? "bg-muted font-medium" : ""
                }`}
                title={v.note || undefined}
              >
                <div className="flex items-center gap-1">
                  <FileTextIcon className="size-3" />
                  v{v.version}
                  {v.parent_id ? <span className="text-muted-foreground">↳</span> : null}
                </div>
                <div className="truncate text-[10px] text-muted-foreground">
                  {new Date(v.created_at).toLocaleString()}
                </div>
              </button>
            ))
          )}
        </CardContent>
      </Card>

      <div className="grid gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <select
            className="h-8 rounded-md border bg-transparent px-2 text-xs"
            value={profileID}
            onChange={(e) => void selectProfile(Number(e.target.value))}
            title={t("formatHint")}
          >
            <option value={0}>{t("formatDefault")}</option>
            {profiles.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
                {p.builtin ? " *" : ""}
              </option>
            ))}
          </select>
          <Button size="sm" variant="outline" onClick={startEditFormat} disabled={busy}>
            <PencilIcon data-icon="inline-start" />
            {t("editFormat")}
          </Button>
          <Button size="sm" variant="outline" onClick={() => void copy()} disabled={!selected}>
            {copied ? <CheckIcon data-icon="inline-start" /> : <CopyIcon data-icon="inline-start" />}
            {t("copy")}
          </Button>
          <Button size="sm" onClick={() => void download()} disabled={!selected}>
            <DownloadIcon data-icon="inline-start" />
            {t("download")}
          </Button>
        </div>

        {editing ? (
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm">{t("editFormatTitle")}</CardTitle>
            </CardHeader>
            <CardContent className="grid gap-2">
              <input
                className="h-8 rounded-md border bg-transparent px-2 text-xs outline-none focus:border-foreground/40"
                value={formatName}
                onChange={(e) => setFormatName(e.target.value)}
                placeholder={t("formatName")}
              />
              <textarea
                className="min-h-48 w-full rounded-md border bg-transparent p-2 font-mono text-xs outline-none focus:border-foreground/40"
                value={formatDraft}
                onChange={(e) => setFormatDraft(e.target.value)}
                placeholder={t("formatInstructions")}
              />
              <div className="flex gap-2">
                <Button size="sm" onClick={() => void saveFormat()} disabled={busy}>
                  {t("formatSave")}
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setEditing(false)} disabled={busy}>
                  {t("cancel")}
                </Button>
              </div>
            </CardContent>
          </Card>
        ) : null}

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">
              {selected ? `v${selected.version}` : t("title")}
              {selected?.note ? <span className="ml-2 text-xs font-normal text-muted-foreground">— {selected.note}</span> : null}
            </CardTitle>
          </CardHeader>
          <CardContent className="max-h-[60vh] overflow-y-auto">
            {error ? (
              <div className="text-sm text-destructive">{error}</div>
            ) : selected?.content ? (
              <DemoMarkdown text={selected.content} />
            ) : (
              <Skeleton className="h-40 w-full" />
            )}
          </CardContent>
        </Card>

        <div className="flex items-end gap-2">
          <textarea
            className="min-h-16 flex-1 rounded-md border bg-transparent p-2 text-xs outline-none focus:border-foreground/40"
            placeholder={t("revisePlaceholder")}
            value={instruction}
            onChange={(e) => setInstruction(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) void revise();
            }}
          />
          <Button size="sm" onClick={() => void revise()} disabled={busy || !instruction.trim() || !selected}>
            <SendIcon data-icon="inline-start" />
            {busy ? t("revising") : t("revise")}
          </Button>
        </div>
      </div>
    </div>
  );
}
