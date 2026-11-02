// Demo mode: shared vocabulary for masking sensitive values.
//
// The on/off switch is per browser (cookie) so the boot script can apply it before the
// first paint; WHAT is masked is an instance setting (classes + custom regex rules) and
// comes from the server.

export const DEMO_CLASSES = ["secret", "identity", "target", "evidence"] as const;
export type DemoClass = (typeof DEMO_CLASSES)[number];

export type DemoSettings = {
  classes: DemoClass[];
  rules: string[];
};

export const DEMO_DEFAULTS: DemoSettings = {
  classes: ["secret", "identity", "target", "evidence"],
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

// maskedValue keeps a length hint for secrets: the last four characters, nothing else.
export function maskedValue(value: string): string {
  if (!value) return "—";
  if (value.length <= 8) return "••••••";
  return `••••••${value.slice(-4)}`;
}
