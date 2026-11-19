// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, test } from "vitest";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

test("leaves positioning to Astryx so a dialog stays in the viewport", () => {
  act(() =>
    root.render(
      <Dialog open>
        <DialogContent className="definition-dialog">
          <DialogTitle>Edit</DialogTitle>
          <DialogDescription>Body</DialogDescription>
        </DialogContent>
      </Dialog>,
    ),
  );
  const dialog = document.querySelector<HTMLElement>(
    '[data-slot="dialog-content"]',
  );
  expect(dialog).not.toBeNull();
  // A positioning utility (relative/absolute/static) overrides Astryx's
  // position: fixed and opens the dialog out of frame on a scrolled page.
  expect(dialog!.className).not.toMatch(
    /(^|\s)(relative|absolute|static)(\s|$)/,
  );
  expect(dialog!.className).toContain("definition-dialog");
});
