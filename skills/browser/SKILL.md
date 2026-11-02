---
name: browser
description: Drive a headless Chromium over CDP with one shell command per step (open a page, read a text snapshot, click, fill, eval, screenshot). Use whenever a target needs a real browser - JS-rendered pages, SPA login/invite flows, fragment-based tokens, screenshots as evidence - instead of guessing from curl output.
---

# Browser (headless Chromium over CDP)

The image ships a pinned Chromium and a small CLI, `/app/skills/browser/cdp.mjs`.
It keeps ONE browser alive on `127.0.0.1:9222`, so every command reattaches to the
same page instead of paying a cold start, and it prints plain text you can read.

Prefer this over the `mcp__playwright__*` tools: it is one command per step, the
output is a compact aria tree, and it cannot drift from the installed browser.

## Start (once per session)

```bash
node /app/skills/browser/cdp.mjs start
```

It is idempotent: if the endpoint is already up it says `already running`.
If it reports chromium missing, the environment is wrong - do not try to install
a browser, report it instead.

## Commands

```bash
node /app/skills/browser/cdp.mjs goto https://target.example/         # navigate + snapshot
node /app/skills/browser/cdp.mjs snapshot                            # aria tree of the whole page
node /app/skills/browser/cdp.mjs snapshot "form"                     # aria tree of one subtree
node /app/skills/browser/cdp.mjs click "text=Sign in"                # click by text
node /app/skills/browser/cdp.mjs click "#submit"                     # click by CSS
node /app/skills/browser/cdp.mjs click "role=button[name='Continue']" # click by role
node /app/skills/browser/cdp.mjs fill "#email" "user@example.com"    # clear + type into a field
node /app/skills/browser/cdp.mjs press Enter                         # key press on the focused element
node /app/skills/browser/cdp.mjs eval "document.title"               # evaluate JS, print the result
node /app/skills/browser/cdp.mjs eval "location.hash"                # read the SPA route
node /app/skills/browser/cdp.mjs screenshot /app/data/tasks/<id>/shot.png
node /app/skills/browser/cdp.mjs url                                 # current URL
node /app/skills/browser/cdp.mjs stop                                # close the browser
```

Selectors are whatever Playwright's `page.locator()` accepts: CSS, `text=...`,
`role=button[name='...']`, `[data-testid=...]`. The snapshot prints the same
roles and names, so read the snapshot first and then click what it shows.

## How to work a page

1. `goto <url>`, read the snapshot. If it looks empty, run `snapshot` once more -
   single-page apps paint after the first paint.
2. Act on what the snapshot shows, one command at a time: `fill`, `click`, `press`.
3. After every action read the snapshot that the command prints. Do not chain
   blind clicks.
4. For anything that only exists in the page's JS (routes, config, tokens in
   memory), use `eval`.
5. Save a screenshot into the task directory when it is evidence for a finding.

## Pitfalls

- **Changing only the URL fragment does not reload the page.** `goto https://h/page#token`
  while already on `https://h/page#other` keeps the old DOM, so the page script never
  re-runs and you read stale text. Add a throwaway query to force a real load:
  `goto "https://h/page/?r=1#token"`.
- The snapshot is an aria tree, not HTML: it lists roles, names and hrefs. If an element
  you expect is missing, the page has not finished rendering - run `snapshot` again.
- `eval` returns JSON for objects; read it, do not print whole pages into the context.

## Fragment and client-side flows

Links like `https://host/invite/#token=...` or `#/login` never send the fragment
to the server: the SPA reads it from `location.hash` and calls the API itself.
That is exactly why a browser is required for invite/registration flows - `curl`
cannot complete them. Navigate to the full URL including the fragment and let the
page do the work.

## Rules

- Navigate only to hosts inside the task's authorised scope. A redirect or a link
  to another host is a reason to stop and report, not to follow.
- Never paste real credentials you did not create for this engagement.
- Evidence is only evidence when it is the real page's real output. Never build
  or edit an HTML page and screenshot it as proof.
- One browser, one session: do not run two `start` calls or two sessions against
  the same profile.
- `stop` when the browser work is finished so the worker is not left holding it.

## Troubleshooting

- `browser is not running` -> run `start` first.
- `chromium not found` -> the image is missing `PLAYWRIGHT_BROWSERS_PATH`; report it.
- A click times out -> take a fresh `snapshot`, the element name probably differs
  from what you assumed.
- The page is a shell with no content -> `eval "document.body.innerHTML.length"`
  to check whether JS ran, then `snapshot` again after a short wait.
