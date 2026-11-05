// Screenshot runner for the ARTEX UI (used by scripts/ui-shot.sh).
// Runs INSIDE the artex image container (--entrypoint node): the image ships
// node + playwright + chromium, so no local browser is needed.
//
// Env: JWT (required) — login token minted by the wrapper; URL — full URL to
// shoot; OUT — output png path inside the container; DEMO=1 — pre-set the demo
// cookie so the boot script masks before first paint; FULL=0 to clip to the
// viewport instead of the full page.

const { chromium } = require("/usr/lib/node_modules/playwright");

(async () => {
  const jwt = process.env.JWT;
  const url = process.env.URL;
  const out = process.env.OUT || "/out/ui-shot.png";
  const demo = process.env.DEMO === "1";
  const full = process.env.FULL !== "0";
  if (!jwt || !url) {
    console.error("JWT and URL are required");
    process.exit(2);
  }

  const browser = await chromium.launch({ args: ["--no-sandbox"] });
  const ctx = await browser.newContext({ viewport: { width: 1600, height: 950 } });
  await ctx.addInitScript(
    ([token, demoOn]) => {
      localStorage.setItem("artex_token", token);
      if (demoOn) {
        // Same shape as demo.ts: the boot script reads this cookie pre-paint.
        document.cookie = "demo_mode=1; path=/; samesite=lax";
        document.documentElement.dataset.demo = "1";
      }
    },
    [jwt, demo],
  );
  const page = await ctx.newPage();
  page.on("console", (m) => console.log("console:", m.type(), m.text().slice(0, 160)));
  page.on("response", (r) => { if (r.status() >= 400) console.log("http", r.status(), r.url().slice(0, 120)); });
  const resp = await page.goto(url, { waitUntil: "domcontentloaded", timeout: 90000 }).catch((e) => {
    console.error("nav:", e.message);
    return null;
  });
  console.log("http:", resp ? resp.status() : "-", "url:", page.url());
  // The UI polls, so networkidle never fires; fixed waits settle hydration.
  // Cold starts can leave the pre-paint splash stuck — detect an empty body
  // and reload once.
  for (let attempt = 1; attempt <= 2; attempt++) {
    await page.waitForTimeout(9000);
    const text = (await page.evaluate(() => document.body?.innerText ?? "")).trim();
    if (text.length > 80 || attempt === 2) break;
    console.log("page looks blank (", text.length, "chars), reloading");
    await page.reload({ waitUntil: "domcontentloaded", timeout: 90000 }).catch(() => {});
  }
  await page.screenshot({ path: out, fullPage: full });
  await browser.close();
  console.log("saved:", out);
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
