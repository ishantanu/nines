// @vitest-environment jsdom
import { test, expect, vi } from "vitest";
import { MaskedReplayCapture } from "./replay-capture";
const mocks = vi.hoisted(() => ({ record: vi.fn() }));
vi.mock("@rrweb/record", () => ({ record: mocks.record }));
test("rrweb masks private copy before emission and uses the finite label catalog", async () => {
  let options: any;
  mocks.record.mockImplementation((o) => {
    options = o;
    return () => {};
  });
  const capture = new MaskedReplayCapture();
  await capture.start(true);
  try {
    const label = document.createElement("span");
    label.setAttribute("data-replay-label", "Home");
    expect(options.maskAllInputs).toBe(true);
    expect(options.maskTextFn("Home", label)).toBe("Home");
    expect(options.maskTextFn("secret", label)).toBe("******");
    expect(options.maskTextFn("Home", document.createElement("h1"))).toBe(
      "****",
    );
    const privateRegion = document.createElement("div");
    privateRegion.setAttribute("data-private", "");
    privateRegion.append(label);
    expect(options.maskTextFn("Home", label)).toBe("****");
    privateRegion.removeAttribute("data-private");
    privateRegion.setAttribute("data-replay-block", "");
    expect(options.maskTextFn("Home", label)).toBe("****");
    expect(options.blockSelector).toContain('input[type="password"]');
  } finally {
    capture.discard();
  }
});
