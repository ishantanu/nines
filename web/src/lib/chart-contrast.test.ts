import { readFileSync } from "node:fs";
import { expect, test } from "vitest";

const css = readFileSync(new URL("../index.css", import.meta.url), "utf8");
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
    const color = (name: string) => {
      const hex = declarations.match(new RegExp(`--${name}: #(\\w+);`))![1];
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
