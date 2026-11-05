import { beforeEach, describe, expect, it } from "vitest";

import { configureDemoMask, maskPayload } from "@/lib/demo";

// The transport masking contract: operator regex rules hit every string,
// field-name denylist values are fully masked, reveal-managed keys are exempt.
// Demo on/off is read from the cookie; tests set it directly on document.

const RULES = ["curiaforum[.]com", "internal-app\\.example"];

function setDemoCookie(on: boolean) {
  document.cookie = `demo_mode=${on ? "1" : "0"}; path=/`;
}

describe("maskPayload", () => {
  beforeEach(() => {
    configureDemoMask(RULES);
    setDemoCookie(true);
  });

  it("applies operator rules to every string in the payload", () => {
    const out = maskPayload({
      findings: [{ summary: "pentest of curiaforum.com auth" }],
      nested: { deep: ["see internal-app.example"] },
    });
    expect(out.findings[0].summary).toBe("pentest of ••• auth");
    expect(out.nested.deep[0]).toBe("see •••");
  });

  it("fully masks denylist field values, keeps empty ones", () => {
    const out = maskPayload({
      api_key: "sk-live-secret",
      email: "",
      nested: { token: "abc.def.ghi" },
    });
    expect(out.api_key).toBe("••••••");
    expect(out.email).toBe("");
    expect(out.nested.token).toBe("••••••");
  });

  it("leaves reveal-managed keys untouched", () => {
    const out = maskPayload({
      name: "curiaforum.com engagement",
      title: "curiaforum.com",
      api_key_hint: "…w3jz",
    });
    expect(out.name).toBe("curiaforum.com engagement");
    expect(out.title).toBe("curiaforum.com");
    expect(out.api_key_hint).toBe("…w3jz");
  });

  it("passes numbers, booleans and null through", () => {
    const out = maskPayload({ n: 42, b: false, z: null, arr: [1, "curiaforum.com"] });
    expect(out.n).toBe(42);
    expect(out.b).toBe(false);
    expect(out.z).toBeNull();
    expect(out.arr[1]).toBe("•••");
  });

  it("does nothing when demo mode is off", () => {
    setDemoCookie(false);
    const out = maskPayload({ summary: "curiaforum.com" });
    expect(out.summary).toBe("curiaforum.com");
  });

  it("does nothing when no rules are configured", () => {
    configureDemoMask([]);
    const out = maskPayload({ summary: "curiaforum.com" });
    expect(out.summary).toBe("curiaforum.com");
  });
});
