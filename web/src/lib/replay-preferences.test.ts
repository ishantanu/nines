import { afterEach, expect, test, vi } from "vitest";
import { initialDarkMode, readPreference, savePreference } from "./preferences";
afterEach(() => vi.unstubAllGlobals());
test("unavailable storage never blocks startup or the theme control", () => {
  vi.stubGlobal("window", {
    get localStorage() {
      throw new Error("denied");
    },
    matchMedia: () => ({ matches: true }),
  });
  expect(readPreference("nines-theme")).toBeNull();
  expect(() => savePreference("nines-theme", "dark")).not.toThrow();
  expect(initialDarkMode()).toBe(true);
});
