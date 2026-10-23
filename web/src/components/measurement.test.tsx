import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { bindingStarter } from "@/lib/topology";
import { measurementDraft } from "@/lib/measurement";
import { MeasurementFields } from "./measurement-editor";
import { MeasurementSummary } from "./measured-objective";

test("measurement form exposes populations and success criteria without requiring query or YAML input", () => {
  const html = renderToStaticMarkup(
    <MeasurementFields
      draft={measurementDraft()}
      connections={[{ id: "primary", kind: "prometheus" }]}
      onChange={() => {}}
    />,
  );
  for (const label of [
    "What do you want to measure?",
    "Population",
    "Success",
    "Source scrape interval",
    "Where is this measured",
  ])
    expect(html).toContain(label);
  expect(html).not.toContain("Good-event counter selector");
  expect(html).not.toContain("YAML manifest");
});
test("latency form requires an explicit threshold and explains histogram limitations", () => {
  const d = measurementDraft();
  d.measurement = "latency";
  const html = renderToStaticMarkup(
    <MeasurementFields draft={d} connections={[]} onChange={() => {}} />,
  );
  expect(html).toContain("Good events complete within (seconds)");
  expect(html).toContain("exact threshold bucket");
  expect(html).not.toContain("Add success filter");
});
test("measurement review shows the exact denominator and numerator, and escapes telemetry labels", () => {
  const b = bindingStarter("primary", "requests_total", {
    environment: "<script>bad</script>",
  });
  const html = renderToStaticMarkup(<MeasurementSummary definition={b} />);
  expect(html).toContain("Eligible events");
  expect(html).toContain("Good events");
  expect(html).toContain("&lt;script&gt;");
  expect(html).not.toContain("<script>");
  expect(html).toContain("status = &quot;200&quot;");
});
