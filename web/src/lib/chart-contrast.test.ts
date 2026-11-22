import { readFileSync } from "node:fs";
import { expect, test } from "vitest";
import { resolveThemeTokens } from "@astryxdesign/core/theme";
import { ninesTheme } from "@/astryx-theme";

const css = readFileSync(new URL("../index.css", import.meta.url), "utf8");
// Legacy variables alias Astryx tokens (see index.css); resolve them for the
// mode under test so the checks run against the colors actually shipped.
const hexOf = (value: string, mode: "light" | "dark") => {
  const alias = value.match(/^var\((--[\w-]+)\)$/);
  const resolved = alias
    ? resolveThemeTokens(ninesTheme, { mode })[alias[1]]
    : value;
  return resolved.replace("#", "").slice(0, 6);
};
const luminance = (rgb: number[]) =>
  rgb
    .map((v) => {
      const s = v / 255;
      return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
    })
    .reduce((sum, value, i) => sum + value * [0.2126, 0.7152, 0.0722][i], 0);
const contrast = (a: number[], b: number[]) => {
  const first = luminance(a),
    second = luminance(b);
  return (Math.max(first, second) + 0.05) / (Math.min(first, second) + 0.05);
};
for (const theme of [":root", ".dark"]) {
  test(`${theme} chart text, series and controls meet WCAG AA contrast`, () => {
    const declarations = css.slice(css.indexOf(theme + " {")).split("}")[0];
    const rootBlock = css.slice(css.indexOf(":root {")).split("}")[0];
    const color = (name: string) => {
      const pattern = new RegExp(`--${name}: (#\\w+|var\\(--[\\w-]+\\));`);
      // .dark only redeclares what differs; the rest cascades from :root.
      const hex = hexOf(
        (declarations.match(pattern) ?? rootBlock.match(pattern))![1],
        theme === ".dark" ? "dark" : "light",
      );
      const full =
        hex.length === 3
          ? hex
              .split("")
              .map((c) => c + c)
              .join("")
          : hex;
      return [0, 2, 4].map((start) =>
        parseInt(full.slice(start, start + 2), 16),
      );
    };
    for (const background of ["card", "background"]) {
      for (const text of [
        "foreground",
        "muted-foreground",
        "destructive",
        "status-degraded",
        "status-healthy",
        "status-at-risk",
        "status-unknown",
      ]) {
        expect(
          contrast(color(text), color(background)),
          `${text} on ${background}`,
        ).toBeGreaterThanOrEqual(4.5);
      }
      for (const graphic of [
        "chart-sli",
        "chart-short",
        "chart-long",
        "ring",
        "input",
        "primary",
        "status-degraded",
        "status-healthy",
      ]) {
        expect(
          contrast(color(graphic), color(background)),
          `${graphic} on ${background}`,
        ).toBeGreaterThanOrEqual(3);
      }
    }
    expect(
      contrast([255, 255, 255], color("destructive-surface")),
      "destructive action text",
    ).toBeGreaterThanOrEqual(4.5);
    expect(
      contrast(color("primary-foreground"), color("primary")),
      "primary action text",
    ).toBeGreaterThanOrEqual(4.5);
  });
}

// Amber notices in dark mode use Astryx warning tokens (index.css). The
// muted fill is translucent, so blend it over the card before measuring.
test("dark amber notices meet WCAG AA contrast", () => {
  expect(css).toContain('.dark [class~="bg-amber-50"]');
  const tokens = resolveThemeTokens(ninesTheme, { mode: "dark" });
  const rgba = (name: string) => {
    const hex = tokens[name].replace("#", "");
    const alpha = hex.length === 8 ? parseInt(hex.slice(6, 8), 16) / 255 : 1;
    return {
      rgb: [0, 2, 4].map((i) => parseInt(hex.slice(i, i + 2), 16)),
      alpha,
    };
  };
  const card = rgba("--color-background-card").rgb;
  const fill = rgba("--color-warning-muted");
  const blended = card.map((c, i) =>
    Math.round(fill.rgb[i] * fill.alpha + c * (1 - fill.alpha)),
  );
  const text = rgba("--color-text-warning").rgb;
  expect(contrast(text, blended)).toBeGreaterThanOrEqual(4.5);
});
