"use client";

import * as React from "react";

import { Markdown } from "@/components/markdown";
import { maskText } from "@/lib/demo";

import { useDemoMode } from "./demo-mode";

// DemoMarkdown renders Markdown with the operator's demo-mode rules applied to the raw
// text first, so masked words never reach the DOM as real values on a demo screen.
export function DemoMarkdown({ text }: { text: string }) {
  const { demo, rules } = useDemoMode();
  const value = React.useMemo(() => (demo && rules.length > 0 ? maskText(text, rules) : text), [demo, rules, text]);
  return <Markdown text={value} />;
}
