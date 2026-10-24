import { test, expect } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { ReplayLabel } from "./replay-label";
import { PageHeader } from "./workspace-chrome";

test("only product-owned headings opt into readable replay labels", () => {
  const privateHeading = renderToStaticMarkup(
    <PageHeader title="Home" description="Private resource" />,
  );
  expect(privateHeading).not.toContain("data-replay-label");
  const publicHeading = renderToStaticMarkup(
    <PageHeader replayTitle title="Home" description="Product page" />,
  );
  expect(publicHeading).toContain('data-replay-label="Home"');
  expect(
    renderToStaticMarkup(<ReplayLabel>Private resource name</ReplayLabel>),
  ).not.toContain("data-replay-label");
});
