import { readFileSync } from "node:fs";
import { expect, test } from "vitest";

const css = readFileSync(new URL("../index.css", import.meta.url), "utf8");

// On wide screens a review shows conversation and evidence side by side. The
// tabs mark the inactive pane [hidden], which base styles force to
// display: none !important, so the override must name it and win.
test("wide review layouts show the evidence pane", () => {
  const wide = css.slice(css.indexOf("@media (min-width: 1280px)"));
  const rule = wide.slice(
    0,
    wide.indexOf("}", wide.indexOf(".investigation-pane[hidden]")),
  );
  expect(rule).toContain(".investigation-pane[hidden]");
  expect(rule).toContain("display: flex !important");
});
