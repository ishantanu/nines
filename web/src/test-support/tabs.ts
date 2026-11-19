// Astryx repeats each tab label in an aria-hidden span to reserve width, so a
// tab's textContent is doubled. Read the visible label instead.
export function tabLabel(tab: Element | null | undefined): string {
  if (!tab) return "";
  return (
    [...tab.querySelectorAll("span")].find(
      (s) =>
        s.children.length === 0 &&
        !!s.textContent &&
        !s.closest("[aria-hidden]"),
    )?.textContent ?? ""
  );
}
export const tabs = (root: ParentNode = document) => [
  ...root.querySelectorAll<HTMLButtonElement>('[role="tab"]'),
];
export const activeTab = (root: ParentNode = document) =>
  root.querySelector('[role="tab"][aria-selected="true"]');
