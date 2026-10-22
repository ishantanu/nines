import { expect, it } from "vitest";
import { telemetryRoute } from "./observability";
it("uses bounded route templates without IDs or query strings", () => {
  expect(
    telemetryRoute("/api/slos/customer-secret/context?token=never-export"),
  ).toBe("/api/slos/{id}/context");
  expect(telemetryRoute("/api/slos/private-id/rules/apply")).toBe(
    "/api/slos/{id}/rules/apply",
  );
  expect(
    telemetryRoute(
      "/api/investigations/private-review/runs/private-run/cancel?token=never-export",
    ),
  ).toBe("/api/investigations/{id}/runs/{run}/cancel");
  expect(telemetryRoute("/api/unknown-secret")).toBe("/api/*");
  expect(telemetryRoute("/api/resources/search?query=secret")).toBe(
    "/api/resources/search",
  );
});
