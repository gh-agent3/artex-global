// Demo mode: shared vocabulary for masking sensitive values.
//
// The on/off switch is per browser (cookie) so the boot script can apply it before the
// first paint; WHAT is masked is an instance setting (classes + custom regex rules) and
// comes from the server.

// Task names are target hostnames in this product, hence their own class (on by default).
export const DEMO_CLASSES = ["secret", "identity", "target", "evidence", "taskname"] as const;
export type DemoClass = (typeof DEMO_CLASSES)[number];

export type DemoSettings = {
  classes: DemoClass[];
  rules: string[];
};

export const DEMO_DEFAULTS: DemoSettings = {
  classes: ["secret", "identity", "target", "evidence", "taskname"],
  rules: [],
};

const COOKIE = "demo_mode";

export function readDemoCookie(): boolean {
  if (typeof document === "undefined") return false;
  const hit = document.cookie.split("; ").find((c) => c.startsWith(`${COOKIE}=`));
  return hit ? hit.slice(COOKIE.length + 1) === "1" : false;
}

export function writeDemoCookie(on: boolean) {
  if (typeof document === "undefined") return;
  document.cookie = `${COOKIE}=${on ? "1" : "0"}; path=/; max-age=31536000; samesite=lax`;
}

// applyDemoAttribute mirrors the cookie onto <html> so CSS can mask prerendered text
// before hydration (and so a screenshot at any moment is already safe).
export function applyDemoAttribute(on: boolean) {
  if (typeof document === "undefined") return;
  document.documentElement.dataset.demo = on ? "1" : "0";
}

// compileRules turns the operator's regex list into RegExp objects, skipping broken
// patterns instead of throwing during a render.
export function compileRules(rules: string[]): RegExp[] {
  const out: RegExp[] = [];
  for (const rule of rules ?? []) {
    try {
      out.push(new RegExp(rule, "gi"));
    } catch {
      /* the server validates on save; a stale/broken rule must not blank the UI */
    }
  }
  return out;
}

export const RULE_MASK = "•••";

// maskText replaces every custom-rule match with a fixed mask. Used on free text that is
// too variable for a field-level component (evidence, report bodies, command output).
export function maskText(text: string, rules: RegExp[]): string {
  if (!text || rules.length === 0) return text;
  let out = text;
  for (const rx of rules) {
    out = out.replace(rx, RULE_MASK);
  }
  return out;
}

// maskedValue is a fixed mask on purpose: keeping the last characters leaked the TLD
// ("customer-portal.example" -> "••••••.com"), which defeats the point.
export function maskedValue(value: string): string {
  return value ? "••••••" : "—";
}

// ── Transport masking ────────────────────────────────────────────────────────
// One masking pass at the client transport choke point (http<T> in api.ts), so
// every surface — findings, traffic, activity streams, task text — is covered
// without per-screen wiring (the arch-review decision; docs/backlog.md item 1).
// Field-name denylist (Sentry-style) for structured secrets; the operator regex
// rules run over every other string. Keys managed by <Sensitive>/task-name
// classes are exempt so click-to-reveal keeps working.

const DENYLIST = /^(password|passwd|pwd|secret|token|access_token|refresh_token|id_token|api[_-]?key|authorization|auth|cookie|set-cookie|credential|credentials|private[_-]?key|session|email|e-mail|invite|invitation)$/i;

// Exempt: reveal-managed fields (LLM key hints, task names/titles) — masking
// them here would break the intentional click-to-reveal.
const EXEMPT = /^(name|title|api_key_hint)$/i;

const MASK_FULL = "••••••";
const MAX_DEPTH = 12;
const MAX_STRING = 200_000;

let transportRules: RegExp[] = [];

// configureDemoMask is called by DemoModeProvider once the instance rules load.
export function configureDemoMask(rules: string[]) {
  transportRules = compileRules(rules);
}

export function demoMaskActive(): boolean {
  return readDemoCookie();
}

export function maskPayload<T>(value: T): T {
  if (!demoMaskActive() || transportRules.length === 0) return value;
  return walk(value, 0) as T;
}

function walk(value: any, depth: number): any {
  if (value === null || value === undefined) return value;
  if (typeof value === "string") {
    if (value.length > MAX_STRING) return value.slice(0, MAX_STRING);
    return maskText(value, transportRules);
  }
  if (typeof value !== "object" || depth > MAX_DEPTH) return value;
  if (Array.isArray(value)) {
    for (let i = 0; i < value.length; i++) value[i] = walk(value[i], depth + 1);
    return value;
  }
  for (const key of Object.keys(value)) {
    const v = value[key];
    if (typeof v === "string") {
      if (EXEMPT.test(key)) continue;
      if (DENYLIST.test(key)) {
        value[key] = v === "" ? v : MASK_FULL;
      } else if (v.length <= MAX_STRING) {
        value[key] = maskText(v, transportRules);
      }
    } else {
      value[key] = walk(v, depth + 1);
    }
  }
  return value;
}
