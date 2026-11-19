import { expect, test } from "vitest";
import { rowsForMinHeight } from "./textarea";

test("a minimum height becomes enough rows to fill it", () => {
  // min-h-80 is 320px; less 16px padding at a 16px line height is 19 rows.
  expect(rowsForMinHeight("min-h-80 font-mono text-xs")).toBe(19);
  expect(rowsForMinHeight("min-h-80 font-mono text-sm")).toBe(15);
  expect(rowsForMinHeight("mt-2 min-h-28 font-mono text-xs")).toBe(6);
  // Small boxes keep Astryx's three-row minimum; no class means no override.
  expect(rowsForMinHeight("h-12 min-h-12 resize-none")).toBe(3);
  expect(rowsForMinHeight("font-mono")).toBeUndefined();
  expect(rowsForMinHeight(undefined)).toBeUndefined();
});
