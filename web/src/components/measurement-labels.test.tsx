import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { MeasurementLabels } from "./measurement-labels";

test("measurement labels have readable terms and safely escaped values", () => {
  const html = renderToStaticMarkup(
    <MeasurementLabels
      labels={{ service: "api", environment: "<production>" }}
    />,
  );
  expect(html).toContain("<dt");
  expect(html).toContain("<dd");
  expect(html.indexOf("environment")).toBeLessThan(html.indexOf("service"));
  expect(html).toContain("&lt;production&gt;");
  expect(html).not.toContain('{"');
});

test("missing success criteria stay explicit instead of suggesting success", () => {
  expect(renderToStaticMarkup(<MeasurementLabels />)).toContain(
    "No label filters",
  );
  expect(
    renderToStaticMarkup(
      <MeasurementLabels empty="No success labels specified" />,
    ),
  ).toContain("No success labels specified");
});
