"use client";

import * as React from "react";

import { usePathname } from "next/navigation";

import { api } from "@/lib/api";
import {
  applyDemoAttribute,
  compileRules,
  DEMO_CLASSES,
  DEMO_DEFAULTS,
  type DemoClass,
  type DemoSettings,
  maskText,
  maskedValue,
  readDemoCookie,
  writeDemoCookie,
} from "@/lib/demo";

// Auto-hide: a revealed value is meant for a glance during a walkthrough, not for the rest
// of the recording. It also hides on route change and when the tab loses focus.
const REVEAL_MS = 15_000;

type DemoState = {
  demo: boolean;
  settings: DemoSettings;
  toggle: () => void;
  isRevealed: (key: string) => boolean;
  reveal: (key: string) => void;
  concealAll: () => void;
  rules: RegExp[];
  enabled: (kind: DemoClass) => boolean;
};

const DemoContext = React.createContext<DemoState | null>(null);

export function DemoModeProvider({ children }: { children: React.ReactNode }) {
  const [demo, setDemo] = React.useState<boolean>(() => readDemoCookie());
  const [settings, setSettings] = React.useState<DemoSettings>(DEMO_DEFAULTS);
  const [revealed, setRevealed] = React.useState<Record<string, number>>({});
  const pathname = usePathname();

  React.useEffect(() => {
    applyDemoAttribute(demo);
  }, [demo]);

  React.useEffect(() => {
    let alive = true;
    api
      .demoSettings()
      .then((s) => {
        if (alive) {
          const classes = (s.classes ?? DEMO_DEFAULTS.classes).filter((c): c is DemoClass =>
            (DEMO_CLASSES as readonly string[]).includes(c),
          );
          setSettings({ classes: classes.length > 0 ? classes : DEMO_DEFAULTS.classes, rules: s.rules ?? [] });
        }
      })
      .catch(() => {
        /* keep the defaults; masking must never depend on the network */
      });
    return () => {
      alive = false;
    };
  }, []);

  // A reveal lives for REVEAL_MS only.
  React.useEffect(() => {
    const keys = Object.keys(revealed);
    if (keys.length === 0) return;
    const timers = keys.map((k) =>
      setTimeout(() => {
        setRevealed((prev) => {
          const next = { ...prev };
          delete next[k];
          return next;
        });
      }, REVEAL_MS),
    );
    return () => timers.forEach(clearTimeout);
  }, [revealed]);

  // Navigating away or leaving the tab re-arms the safe state.
  React.useEffect(() => {
    setRevealed({});
  }, [pathname]);
  React.useEffect(() => {
    const onHide = () => setRevealed({});
    document.addEventListener("visibilitychange", onHide);
    window.addEventListener("blur", onHide);
    return () => {
      document.removeEventListener("visibilitychange", onHide);
      window.removeEventListener("blur", onHide);
    };
  }, []);

  const toggle = React.useCallback(() => {
    setDemo((prev) => {
      const next = !prev;
      writeDemoCookie(next);
      setRevealed({});
      return next;
    });
  }, []);

  // Ctrl/Cmd+Shift+D toggles without hunting for the button.
  React.useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key.toLowerCase() === "d") {
        e.preventDefault();
        toggle();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [toggle]);

  const value = React.useMemo<DemoState>(
    () => ({
      demo,
      settings,
      toggle,
      isRevealed: (key: string) => Boolean(revealed[key]),
      reveal: (key: string) => setRevealed((prev) => ({ ...prev, [key]: 1 })),
      concealAll: () => setRevealed({}),
      rules: compileRules(settings.rules),
      enabled: (kind: DemoClass) => settings.classes.includes(kind),
    }),
    [demo, settings, toggle, revealed],
  );

  return <DemoContext.Provider value={value}>{children}</DemoContext.Provider>;
}

export function useDemoMode(): DemoState {
  const ctx = React.useContext(DemoContext);
  if (!ctx) throw new Error("useDemoMode must be used inside DemoModeProvider");
  return ctx;
}

type SensitiveProps = {
  kind: DemoClass;
  children: React.ReactNode;
  /** Rendered text, used for the length hint and the copy guard. */
  value?: string;
  /** Block content (evidence, command output) is replaced by a placeholder line. */
  block?: boolean;
  className?: string;
  /** Stable key for the reveal state; defaults to a per-instance id. */
  revealKey?: string;
};

// Sensitive masks one value while demo mode is on. A click reveals just this value for a
// few seconds; turning demo mode off reveals everything.
export function Sensitive({ kind, children, value, block, className, revealKey }: SensitiveProps) {
  const { demo, enabled, isRevealed, reveal } = useDemoMode();
  const auto = React.useId();
  const key = revealKey ?? `${kind}:${auto}`;
  const hidden = demo && enabled(kind) && !isRevealed(key);

  if (!hidden) {
    return <span className={className}>{children}</span>;
  }

  if (block) {
    return (
      <button
        type="button"
        className="my-1 w-full rounded-md border border-dashed px-2 py-1 text-left text-xs text-muted-foreground hover:bg-muted/50"
        title="Hidden in demo mode - click to reveal"
        onClick={() => reveal(key)}
      >
        •••••• hidden in demo mode - click to reveal
      </button>
    );
  }

  return (
    <button
      type="button"
      className="cursor-pointer rounded-sm font-mono text-muted-foreground hover:bg-muted/60"
      title="Hidden in demo mode - click to reveal"
      onClick={() => reveal(key)}
    >
      {value ? maskedValue(value) : "••••••"}
    </button>
  );
}

// DemoText masks free text with the operator's regex rules (evidence blocks, command
// output, report bodies). Fields that fit <Sensitive> should use that instead.
export function DemoText({ text, block }: { text: string; block?: boolean }) {
  const { demo, rules } = useDemoMode();
  const masked = demo && rules.length > 0 ? maskText(text, rules) : text;
  if (block) {
    return (
      <pre className="overflow-x-auto rounded-md bg-muted p-2 text-xs whitespace-pre-wrap">{masked}</pre>
    );
  }
  return <span>{masked}</span>;
}
