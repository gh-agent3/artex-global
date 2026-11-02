# Demo mode: hide sensitive data, reveal on demand

Planning note. Nothing here is implemented yet.

## Goal

One eye button in the header turns the whole UI into a demo-safe view: every sensitive
value is masked. The operator can unmask one field by clicking it, or unmask everything
by turning demo mode off. It has to survive a screen share, a screenshot and a video
recording of a live walkthrough.

## Non-goals

- It is not an access control. The real values are still in the API responses and in the
  DOM; a person with devtools can read them. Demo mode protects the SCREEN, not the data.
- It does not change exports (report, CSV, archives). Those keep real values; the UI
  warns once when demo mode is on and the operator downloads something.

## Layers

1. **State** - one boolean, owned by a provider mounted in `(main)/layout.tsx`, persisted
   in the client cookie `demo_mode=1`. Cookie, not localStorage, because the boot script
   can read it before the first paint.
2. **Masking** - a value-level decision, not a global blur. Each sensitive value renders as
   a mask (`••••••` for short values, a placeholder row for blocks) while
   `html[data-demo="1"]`.
3. **Reveal** - clicking a masked value reveals just that value for this session, with an
   auto-hide timer. Turning the eye off reveals everything; turning it on hides everything
   again and forgets the per-field reveals.

## No flash on load

Theme and layout already work this way: `ThemeBootScript` reads cookies in `<head>` and sets
attributes on `<html>` before paint. Demo mode reuses that:

- `ThemeBootScript` (or a sibling `DemoBootScript`) sets `document.documentElement.dataset.demo`
  from the `demo_mode` cookie.
- CSS does the first mask: `html[data-demo="1"] .demo-mask { ... }`. The prerendered HTML is
  never visible unmasked, because the attribute lands before the first paint.
- React reads the same cookie for its initial state, so the first client render agrees with
  the DOM. No flicker in either direction.

## What is sensitive

Four classes, each switchable in settings (default: all on):

- **secret** - API keys and key hints, bearer tokens, session cookies, passwords, webhook
  secrets. Masked as `••••••` with the length hint kept (`sk-••••3f9a` is still too much;
  keep only the last 4 characters and only when the field is revealed once).
- **identity** - usernames, e-mails, account names, the operator's own account in the header.
- **target** - hostnames, IPs and URLs of the engagement target. For a client demo the target
  is usually the thing that must not leak.
- **evidence** - PoC blocks, command lines and command output, captured request/response
  bodies. These are blocks, not single values: masked with a one-line placeholder.

Classification lives next to the render site (`<Sensitive kind="secret">`), not in a global
regex. A regex over free text (chat, transcripts, agent output) is a phase-3 nice-to-have:
too many false positives and false negatives to trust in a demo.

## Component API

```tsx
<DemoModeProvider>            // mounted once in (main)/layout.tsx
  <Sensitive kind="secret" hint={keyHint}>{value}</Sensitive>
  <Sensitive kind="evidence" block>{poC}</Sensitive>
</DemoModeProvider>
```

- `useDemoMode()` returns `{ demo, toggle, revealed, reveal(key), concealAll }`.
- `<Sensitive>` renders children as-is when demo is off or this key is revealed; otherwise
  the mask. The mask keeps the layout (same width class, same line height).
- Reveal is keyed by a stable string (`secret:llm:3`, `evidence:finding:42`) so re-renders do
  not lose it.
- Auto-hide after 15 s, and immediately on route change, on tab blur, or when demo mode is
  turned back on.

## Header and reachability

- Eye button in the global header (`main-content.tsx`), next to the theme switcher.
- Task detail pages are full-bleed and do NOT render the global header, so the same button
  also goes into the task detail header, next to Pause/Archive.
- Keyboard shortcut `Ctrl/Cmd+Shift+D` toggles it without hunting for the button.
- A small "Demo mode" pill stays visible while it is on, so nobody records a full walkthrough
  without noticing.
- State is per browser and cleared on logout.

## Edge cases that must be decided up front

- **Copy buttons**: copying a masked value is refused (otherwise the clipboard, and any
  clipboard-history UI, leaks it). Copy is allowed only for revealed values.
- **`title=` tooltips and toasts**: a tooltip can show the real value over a masked field.
  The component masks the title too.
- **Downloads and exports**: unaffected, with one warning toast while demo mode is on.
- **Empty and null values**: render `—`, never a mask, so a demo does not look broken.
- **Long values**: truncate and mask, never wrap, so the layout does not shift between
  masked and revealed states.

## Phases

**Phase 1 - skeleton (half a day)**

- `src/lib/demo.ts`: classes, cookie read/write, mask helpers.
- `src/components/demo-mode.tsx`: provider, `useDemoMode`, `<Sensitive>`.
- `src/scripts/theme-boot.tsx`: set `data-demo` before paint.
- `globals.css`: the `data-demo` mask rules.
- Eye button in `main-content.tsx` and in the task detail header.
- i18n keys in all six locales.
- Wrap the obvious first targets: LLM key hints, traffic viewer headers/bodies, finding
  evidence blocks, the account name in the header.

**Phase 2 - coverage**

- Assets, findings tables, report body, chat and transcript blocks.
- Settings switches per class.
- Shortcut, "Demo mode" pill, auto-hide timer.

**Phase 3 - optional, only if the HTML itself must be safe**

- A `?redact=1` request flag the API honours by returning redacted fields, so nothing
  sensitive is ever in the DOM or the network tab. This is the only variant that survives
  someone opening devtools.

## Test plan

- Unit: mask/reveal state machine, auto-hide, cookie round-trip, per-kind switches.
- Component: `<Sensitive>` renders the mask when `data-demo=1`, reveals on click, refuses
  copy while masked, masks the `title`.
- Visual: screenshot with demo on must contain no value from the fixture set (assert on the
  text content, not pixels).
- Regression: with demo off, no mask element is rendered at all.

## Open questions

1. Should the target host (`target` class) be masked by default, or only when the operator
   turns that class on? Masking it by default makes the demo safer but also less useful.
2. Should revealing one field survive a page refresh? Recommendation: no - a refresh during
   a recording should return to the safe state.
3. Do we need a separate "presentation" preset that also hides the sidebar and system pages?
