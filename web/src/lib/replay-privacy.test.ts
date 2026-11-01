import { afterEach, expect, test, vi } from "vitest";
import { createTenantSession } from "./api";
import { replayPrivacy } from "./replays";

afterEach(() => vi.unstubAllGlobals());

test("replay notice requires the current tenant and displayed consent version", async () => {
  const scope = createTenantSession("token", "alpha", "owner");
  const notice = {
    apiVersion: "nines.dev/v1alpha1",
    tenant: "alpha",
    noticeVersion: "replay-sharing-v1",
    enabled: true,
    policyURL: "",
    contact: "",
  };
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  for (const bad of [
    { ...notice, tenant: "beta" },
    { ...notice, noticeVersion: "replay-sharing-v2" },
    { ...notice, apiVersion: "nines.dev/v2" },
    { ...notice, enabled: "true" },
  ]) {
    fetcher.mockResolvedValueOnce(new Response(JSON.stringify(bad)));
    await expect(replayPrivacy(scope)).rejects.toThrow(
      "privacy notice or tenant mismatch",
    );
  }
  fetcher.mockResolvedValueOnce(new Response(JSON.stringify(notice)));
  await expect(replayPrivacy(scope)).resolves.toEqual(notice);
  scope.close();
});
