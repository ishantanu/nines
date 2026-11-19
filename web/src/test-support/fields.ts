import { act } from "react";

// Astryx number and date/time fields commit typed text on blur or Enter, so a
// test "types" by setting the value, firing input, then leaving the field.
export async function typeInto(
  node: HTMLInputElement | HTMLTextAreaElement,
  value: string,
) {
  const proto =
    node instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype;
  await act(async () => {
    node.focus();
    Object.getOwnPropertyDescriptor(proto, "value")!.set!.call(node, value);
    node.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await act(async () => {
    node.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    node.blur();
  });
}

// A date-time field is a date input followed by a time input (minute
// resolution). `iso` is "YYYY-MM-DDTHH:mm".
export async function typeDateTime(id: string, iso: string) {
  const date = document.getElementById(id) as HTMLInputElement;
  const time = date
    .closest("[style*='--x-width']")!
    .querySelectorAll<HTMLInputElement>("input")[1];
  const [d, t] = iso.split("T");
  await typeInto(date, d);
  await typeInto(time, t.slice(0, 5));
}

// Astryx Selector: open the combobox and read or click the options of the
// listbox it controls.
function listbox(trigger: HTMLElement) {
  return document.getElementById(trigger.getAttribute("aria-controls") ?? "")!;
}
export async function optionLabels(id: string | HTMLElement) {
  const trigger = typeof id === "string" ? document.getElementById(id)! : id;
  await act(async () => trigger.click());
  const labels = [...listbox(trigger).querySelectorAll('[role="option"]')].map(
    (o) => o.textContent!.trim(),
  );
  await act(async () => {
    trigger.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
    );
    listbox(trigger).dispatchEvent(new Event("cancel", { bubbles: true }));
  });
  return labels;
}
export async function chooseOption(id: string | HTMLElement, label: string) {
  const trigger = typeof id === "string" ? document.getElementById(id)! : id;
  await act(async () => trigger.click());
  const option = [
    ...listbox(trigger).querySelectorAll<HTMLElement>('[role="option"]'),
  ].find((o) => o.textContent!.trim() === label);
  if (!option) throw new Error(`No option "${label}"`);
  await act(async () => option.click());
}
