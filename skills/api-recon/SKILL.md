---
name: api-recon
description: Use this skill when collecting a website's API endpoints.
---

# API Recon (frontend interface reconnaissance)

Under an **authorised** premise, discover as completely as possible: **backend APIs** (paths, methods, parameters, response bodies), **frontend routes**, and **UI feature trigger points** (tabs, modals, table operations, etc.).

---

## Boundaries and Prohibitions (agent must read; violating them is out of scope)

This skill **does API / parameter-surface reconnaissance only**; it is not a vulnerability-hunting or exploitation stage.

### Task boundaries

| Scope | Allowed | Forbidden |
|---|---|---|
| **Target** | Enumerate path, method, parameters, routes, UI trigger points | SQLi/XSS/IDOR/brute force/fuzz vulnerabilities, packet-tampering attacks, destructive operations |
| **Authentication** | Hook + stub/mock to bypass the **client-side** login gate | Asking the user for or guessing account credentials; attempting real login-form submission |
| **Runtime** | Hook endpoints without credentials, use mock responses to get the SPA into the post-login shell | Flows that can only continue with a real backend session |

### Credential-free dynamic analysis (Phase 3 default)

1. **Intercept and stub** bootstrap endpoints such as login, permissions and menus via `preload.js` / `runtime_harvest.js`;
2. For business query endpoints, return a mock body that is **structurally correct, carries a successful business code, and may contain empty data**;
3. Make the frontend render post-login pages even with no backend or in a 401 environment, so that more XHR/fetch/WebSocket traffic is triggered;
4. **Empty data, blank tables and placeholder UI are all expected** — do not switch to real login or vulnerability testing because of them.

**In one sentence**: use mocks to prop open frontend routes and component mounting, and **record outbound requests only**; what the backend returns does not matter, what matters is **which further endpoints the frontend will send**.

### Hard workflow prohibitions

| Prohibition | Instead do |
|---|---|
| Before Phase 1 completes, grep/curl/Read the main entry `index-*.js` to extract API paths | Run `OUTDIR/harvest_static.py` |
| Hand-writing scripts such as `extract_apis.py` to replace harvest | Modify `OUTDIR/harvest_static.py` and re-run it |
| Repeating the same grep/command after it has failed >=2 times | Change strategy: read tool_logs, modify harvest, consult the reference |
| Skipping gate A/B and running the original `scripts/` directly | Copy into OUTDIR and adapt to the target |
| Real usernames/passwords, OTP, OAuth and other authentication | stub/mock (see above) |
| Skipping stubs "to get real data" and doing IDOR/injection tests | Record outbound only; that is the recon boundary |
| Irreversible operations such as deleting, exporting sensitive data or bulk writes | The same applies to coverage clicking |
| Claiming all pages and endpoints have been obtained without completing runtime + dynamic enumeration | See "Definition of done" or flag the limitation |
| Claiming all parameters are known without completing the parameter trigger matrix + diff | Phase 3b matrix + Phase 5 diff |
| Inferring required/optional from a single runtime sample | Multi-sample diff, or validation rules/error inference |

---

## Two-layer model + run modes

| Layer | Output | Limits |
|---|---|---|
| **Static** (JS bundle) | Full set of endpoint paths, route draft, candidate fields at payload-assembly sites | No HTTP method; parameters require Phase 1b; misses URLs concatenated at runtime |
| **Runtime** (live session) | Method + body + response + dynamic URLs + WS/SSE; multi-sample diff completes the parameters | A page must actually render before it sends requests; a single sample cannot determine required/optional |

| Run mode | Engine | Use for |
|---|---|---|
| **depth** | `runtime_harvest.js` (Puppeteer) | API inventory, METHOD/params/response body, WS/SSE, reproducible batch runs |
| **coverage** | browser + `preload.js` | Click tabs/modals/tables, deeper feature coverage |
| **both** | depth first, then coverage | Most complete, longest runtime |

**Parameter methodology** (no generic script): use harvest/regex for paths; for parameters use **anchor window expansion + UI binding chain + multi-sample diff + error inference** (grep recipes in [reference.md](reference.md) section J).

---

## Definition of done

recon may be claimed complete only when all of the following hold:

- [ ] **Static**: Phase 1 harvest produced `api_static.txt`, `routes.txt`, `js/`
- [ ] **Runtime**: at least one of depth or coverage; coverage/both require **working hooks + dynamic enumeration loop**
- [ ] **Shell entry**: visiting a business path does not land on `/login` (watch for hash routing)
- [ ] **Parameters**: coverage/both completed the parameter trigger matrix + `param_samples.json`; Phase 5 merged `params_merged.json`
- [ ] **Depth** (if module pages are blank): Phase 4 permission-tree reconstruction and re-run, until **module-level APIs** appear (not just locale/bootstrap)
- [ ] **Delivery**: Phase 5 outputs complete (see the Phase 5 output table); `insert_assets` writes the service and endpoint assets

---

## Scripts and gates

`scripts/` holds reference templates only; running the originals directly and treating them as the final result is **forbidden**.

**Rule**: read first → adapt to the target → write into `OUTDIR` (e.g. `recon/`) → record in `CHANGES.md`; if it does not match, rewrite it per the methodology and borrow only the structure.

| Gate | When | Reference script → OUTDIR copy | Common required changes |
|---|---|---|---|
| **A (static)** | After Phase 0, before the **first** harvest/spider run | `harvest_static.py` / `spider_mpa.py` | **On most sites the default regex runs as-is**; change the endpoint regex, webpack/Vite `publicPath`, or MPA exclude/cookie only when the manifest/dialect does not match |
| **B (runtime)** | After Phase 2, before running depth/coverage | `runtime_harvest.js` / `preload.js` + `config.json` | Cookie/localStorage keys, neutralize success value, stubs, login regex, api prefix, hash/history |

**Mandatory SPA order** (not interchangeable; Phase numbering takes precedence over "explore first, script later"):

| Step | Required | Forbidden |
|---|---|---|
| After Phase 0 completes | Next Bash call = `python3 OUTDIR/harvest_static.py <URL> OUTDIR` | curl/grep/Read the main entry `index-*.js` (usually >500KB) |
| Gate A | Copy the script → small changes as needed → **run immediately** | Manually extracting APIs first and then deciding whether to harvest |
| Before Phase 1 completes | Verify outputs with `wc -l`; on 404, modify harvest and retry | Hand-writing extract scripts; repeatedly grepping URLs that were never downloaded |
| From Phase 1b on | grep `OUTDIR/js/*.js` only | Using the main bundle instead of harvest |

- ✅ Copy `harvest_static.py` → (optionally) change the regex → **run immediately**
- ❌ curl the main bundle → grep several times → write a temporary extract → harvest last
- **MPA**: after Phase 0 the next Bash call = `python3 OUTDIR/spider_mpa.py ...`

---

## Tool and output constraints

| Constraint | Description |
|---|---|
| Large files | **Never** Read/grep `index-*.js` over 100KB into context; batch-process with OUTDIR scripts |
| grep output | Must pipe through `\| head -20` or `-m 5`; keep only path summaries in the conversation, never paste bundle fragments |
| Verification | Use `wc -l`, `ls \| wc -l`; do not Read whole directories |
| regex first probe | Optional, <=1 time, only on small chunks <=50KB or HTML; the formal static pass is harvest |
| reference | Recipes/templates/troubleshooting in [reference.md](reference.md); do not duplicate its full text inline |

---

## Execution roadmap

```
Phase 0 classify + OUTDIR
  → Gate A → Phase 1 harvest (★ run immediately ★)
  → Phase 1b parameter reverse engineering
  → Phase 2 the three auth gates → config.json
  → Gate B → Phase 3 runtime + parameter matrix
  → Phase 4 permission tree (if needed) → re-run Phase 3
  → Phase 5 merge report + insert_assets bulk-inserts every discovered service and endpoint api asset; under no circumstances may a discovered asset be missed at insert time
```

Tick in order; **you may not enter the next Phase until the previous item is complete**.

1. [ ] **Phase 0**: probe SPA/MPA; create `OUTDIR` → [Phase 0](#phase-0-classification)
2. [ ] **Gate A + Phase 1**: copy the script → harvest **immediately** → verify with `wc -l` → [Phase 1](#phase-1-static)
3. [ ] **Phase 1b**: anchor window expansion + binding layer → `param_candidates.json` → [Phase 1b](#phase-1b-parameter-reverse-engineering)
4. [ ] **Phase 2**: the three auth gates → `config.json` → [Phase 2](#phase-2-the-three-auth-gates)
5. [ ] **Gate B**: adapt the runtime scripts → [Phase 3](#phase-3-runtime)
6. [ ] **Phase 3**: depth / coverage / both; confirm shell entry; parameter trigger matrix → `param_samples.json`
7. [ ] **Phase 4** (if needed): permission tree → patch stubs → re-run Phase 3 → [Phase 4](#phase-4-permission-tree-reconstruction)
8. [ ] **Phase 5**: merge outputs + report + `insert_assets` → [Phase 5](#phase-5-merge-and-report)

---

## Phase 0: Classification

Fetch the entry HTML and **create `OUTDIR`** (do not modify the skill's own `scripts/`):

- **SPA**: empty shell + `<div id=app>` + chunks → Phase 1–5
- **MPA**: SSR + `<form>`, no endpoint bundle → after Gate A:

```bash
python3 recon/spider_mpa.py <BASE_URL> <OUTDIR> [--cookie "session=..."] [--max 300] [--depth 5] [--exclude "logout|delete|destroy"]
```

Produces `forms.txt`, `links.txt`, `api_inline.txt`. If an SPA yields forms ≈ 0 → switch to Phase 1.

---

## Phase 1: Static

Follow [Scripts and gates](#scripts-and-gates) · [Tool and output constraints](#tool-and-output-constraints).

```bash
python3 recon/harvest_static.py <BASE_URL> <OUTDIR>
```

harvest: parse the HTML scripts → webpack/Vite manifest → download every lazy chunk → produce `js/`, `api_static.txt`, `routes.txt`, `chunkmap.txt`.

```bash
wc -l OUTDIR/api_static.txt OUTDIR/routes.txt
ls OUTDIR/js | wc -l
```

- chunk count vs manifest: on 404 you must modify harvest and retry; do not curl chunks one by one by hand
- `api_static.txt` too small → relax the endpoint regex inside OUTDIR and re-run (see reference)

### Phase 1b: Parameter reverse engineering

Paths come from Phase 1; parameter fields need a separate recon. For grep rules see [Tool and output constraints](#tool-and-output-constraints).

**Completion criterion**: for important endpoints you can answer — field names, transport location, type inference, required or not, sample values, confidence.

#### 1b.0: Transport shape

| Shape | Where the parameters live | Look at statically first |
|---|---|---|
| REST JSON | body + query | `(params\|data\|body)\s*:\s*\{` next to the path anchor |
| GraphQL | `variables` | gql templates, `$page: Int` |
| Classic form | urlencoded | `<form>`, `FormData` |
| File upload | multipart | `FormData.append` |
| Path parameters | `/user/:id` | route table + `useParams` / `$route.params` |
| Encrypted/signed | wrapped into `sign`/`data` | hook the encryption function's arguments (reference section D) |

Output: annotate every endpoint with `transport: query|json|form|graphql|encrypted`.

#### 1b.1: Anchor window expansion

Use a known path as the anchor and expand a window to find the payload-assembly object:

```bash
grep -n '"/api/user/list"' OUTDIR/js/*.js | head -20
grep -rhoaE '.{0,120}("/api[^"]+").{0,200}' OUTDIR/js/*.js | head -20
grep -rhoaE '(params|data|body|payload)\s*:\s*\{' OUTDIR/js/*.js | head -20
```

| Wrapper layer | Parameter clue |
|---|---|
| axios instance | `data` / `params` |
| unified request | interceptors inject global fields |
| OpenAPI client | generated method signatures |
| React Query / SWR | the hook's second argument |
| Vue composable | the composable's arguments |

Type residue: `yup`/`zod`/rules, `Form.Item name=`, embedded Swagger.

→ `param_candidates.json`: `{ path, fields[], source: "static-callsite", confidence }`

#### 1b.2: Binding layer

```
Form field → onFinish/handleSubmit → transform → API payload
```

| Binding source | Technique |
|---|---|
| Form submit | follow submit → transform → API |
| Table search | `getFieldsValue()` → `params` |
| Route | `:id` / `?tab=` |
| Interceptor | global `tenantId`, pagination, sign |
| Enum select | `options` → API enum values |

In the DevTools call stack, walk up from `fetch`/`XHR.send` to the payload-assembly function.

#### 1b.3: The three assembly questions (not the Phase 2 auth gates)

| Question | What to answer |
|---|---|
| **Assembly** | where the payload is built, traces of transform |
| **Validation** | required, pattern, enum |
| **Transport** | path / query / body / multipart / headers |

The interceptor gate (Phase 2) also reads the globally injected fields along the way (Authorization, `X-Tenant-Id`, sign).

#### 1b.4: Handoff to Phase 3

Candidate fields come from static analysis/the binding layer; **required/optional/conditionally dependent** must come from the Phase 3 parameter matrix + diff + Phase 5 error inference.

---

## Phase 2: The three auth gates

grep in `OUTDIR/js/` (with `head`) and write `config.json` (recipes in reference):

| Gate | Question | Keywords |
|---|---|---|
| **Render gate** | How is logged-in state determined? | `isLogin`, `getToken`, Cookie/localStorage |
| **Interceptor gate** | What triggers a redirect to `/login`? | `response_code`, `errno`, axios interceptor |
| **Content gate** | Where do menus/permissions come from? | `menu`, `permission`, `role`, `acl`, `routes` |

Do not treat a localStorage key name as a credential — confirm it from the chunk/request chain.

**Exit = Gate B**: conclusions land in `config.json`, and `OUTDIR/runtime_harvest.js` / `preload.js` are modified.

### Phase 2b: API observation (optional)

Use `preload.js` inside OUTDIR to confirm session key names, Authorization, nested API URLs:

| Config | Output |
|---|---|
| `recordDetail: true` | `__API_RECON_DETAIL__` |
| `observe.xhrHeaders: true` | header observation |
| `extractUrlsFromResponse: true` | child APIs inside responses |
| `observe.storageReads/cookieReads: true` | backfill config |
| `neutralizeVueRouter: true` | `__API_RECON_ROUTES__` |

Export after every coverage round: `__API_RECON_LOG__`, `__API_RECON_DETAIL__`, `__API_RECON_ROUTES__`, `__API_RECON_OBSERVE__`.

---

## Phase 3: Runtime

Gate B must already be passed; comply with [Boundaries and Prohibitions](#boundaries-and-prohibitions-agent-must-read-violating-them-is-out-of-scope) · the credential-free mock strategy.

In `config.json` set `"runtimeMode": "depth" | "coverage" | "both"` (template in reference).

### Hooks and stubs (shared by depth and coverage)

| Layer | Scope | Purpose |
|---|---|---|
| L1 exact | auth/permission/bootstrap stubs | pass first-screen authentication |
| L2 negative correction | all JSON responses | not-logged-in code → success |
| L3 fallback | `/api` and similar not matched by L1 | empty success body, prop open the UI |

- **depth**: fake auth + `forward` rewriting the business code + `stubs`; iterate `routes` (hash/history); produce `runtime_api.json`
- **coverage**: inject `preload.js` at **document-start** (CDP `addScriptToEvaluateOnNewDocument` or a Userscript)

Verify: `window.__API_RECON_PRELOAD__` exists; a business path does not return to `/login`.

```bash
cd recon && npm install
node runtime_harvest.js config.json
```

### 3b: coverage dynamic enumeration (required)

1. Main nav/sidebar — click every item, wait 1–3s for the network
2. Tabs — `role=tab`, `.ant-tabs-tab`
3. Tables — view/edit/detail on the first row
4. Toolbar — export, filter, create (**avoid irreversible deletion**)
5. On entering each module — merge APIs/routes
6. SPA — controlled `pushState` for paths in `routes.txt` not yet covered (forbidden on MPA)

**Parameter trigger matrix** (required): for each module record once per operation type, then **diff the samples**:

| Operation | Parameters usually added |
|---|---|
| First list page | pagination + default filters |
| Click search | keyword, filter |
| Advanced filter | more optional |
| Create/edit | full entity |
| Batch/export/sort | `ids[]`, `exportType`, `sortField` |

**Under stubs the outbound body/headers are still real** — trust the request. Record → `scan_raw.json`, `param_samples.json`, `api_detail.json`.

- **Vue**: `neutralizeVueRouter: true` + document-start preload
- **React**: `routes.txt` + sidebar clicks + `pushState`
- **both**: 3a depth first, then 3b coverage

---

## Phase 4: Permission-tree reconstruction

**Trigger**: module pages are blank / every route returns only bootstrap (e.g. locale) → the content gate was not passed.

| Symptom | Meaning |
|---|---|
| Shell entered | render gate + interceptor gate passed |
| Sidebar items missing / clicks render blank | stub shape or permission codes incomplete |
| Same and very few APIs per route | `v-if permission` not satisfied |
| `routes.txt` far smaller than the bundle | must be completed from the auth module |

```bash
grep -rhoaE '"/api[^"]*(permission|perm|role|menu|acl)[^"]*"' OUTDIR/js/*.js | sort -u | head -30
grep -rhoaE 'userRouteAuth|getResultTree|routeMap|routeLink|menuList|authList' OUTDIR/js/*.js | head -20
```

Typical chain: `role_permissions` (flat codes) + `permissions/all` (tree) → `getResultTree` → `userRouteAuth[CODE].url`.

```bash
python3 recon/extract_route_map.py recon/js recon/
python3 recon/build_perm_tree.py recon/js recon/ --config recon/config.json
```

Intermediate outputs: `route_map.json`, `userRouteAuth.json`, `permissions_tree.json`, `*_stub.json`, `perm_codes_all.txt`.

Stub checks: the outer `response_code` matches the interceptor gate; flat codes align with the tree; `routes` covers every link in `route_map`.

After updating `config.json`, **re-run Phase 3**. For large SPAs you can tune `waitUntil`, `routeTimeout`, `perRouteMs` (see reference sections A3/I).

---

## Phase 5: Merge and report

### Output table

| File | Stage | Content |
|---|---|---|
| `js/`, `api_static.txt`, `routes.txt`, `chunkmap.txt` | 1 | static bundles and paths |
| `param_candidates.json` | 1b | static parameter field candidates |
| `config.json` | 2 | the three gates + runtime config |
| `runtime_api.json` | 3a | depth detailed recording (including WS/SSE) |
| `param_samples.json`, `scan_raw.json`, `api_detail.json` | 3b | samples, click log, detail |
| `route_map.json` and others | 4 | permission-tree intermediate files (if performed) |
| `params_merged.json` | 5 | merged parameter fields + confidence |
| `api_merged.txt` | 5 | `METHOD /path [params] [static\|runtime\|both]` |
| `site_map.json` | 5 | routes, APIs, params, feature points, limitations |
| **insert_assets** | 5 | write all service and endpoint assets into the asset library |

### 5b: Parameter merge

Diff from `param_samples.json`; **there is no generic merge script**. For confidence rules see reference J7 (high/medium/low/pending trigger).

### 5c: Error inference

Within the authorised scope you may send incomplete requests to read 400s (**this is parameter recon, not vulnerability testing**): `field 'x' is required`, enum errors, and so on. Watch for the `data` wrapper, `variables`, and `bizData` before encryption.

The report must state: runtimeMode, static/runtime API counts, parameter confidence, uncovered modules, and a summary of `CHANGES.md` relative to the reference scripts.

Suggested `site_map.json` structure:

```json
{
  "site": "https://example.com",
  "runtimeMode": "both",
  "appType": "vue-spa",
  "routeGuardStrategy": ["nav-neutralize", "L1-auth", "L2-patch", "forward"],
  "apisFromStatic": [],
  "apisFromRuntime": [],
  "apis": [],
  "params": [{ "method": "POST", "path": "/api/user/list", "transport": "json", "fields": [] }],
  "frontendRoutes": [],
  "routesVerifiedByClick": [],
  "featuresTriggered": [],
  "limitations": ""
}
```

More fields and grep recipes in [reference.md](reference.md).

---

## General notes

- **Framework-agnostic**: webpack/Vite/Angular lazy-load methods are the same
- **Transport**: REST/JSON, GraphQL, WebSocket, SSE; gRPC-web is out of scope
- **SSR**: client-side fetch can be recorded; RSC/Server Actions are not fully enumerable
- **Blind spots**: JSVMP, WASM, strong HMAC/mTLS validation → static only + flag the limitation
- **Parameter blind spots**: conditional linkage, hidden params, WASM payload assembly → "pending trigger"/"unreachable"
- **Static is the safety net**: when runtime is blocked, static analysis still enumerates endpoints

---

## Additional resources

- Grep recipes, `config.json` template, troubleshooting, hooks, parameter reverse engineering section J, site_map template: **[reference.md](reference.md)**
- Reference script paths are in the [Scripts and gates](#scripts-and-gates) table
