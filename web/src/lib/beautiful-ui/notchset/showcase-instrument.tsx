// Scoped, silent helpers for the MIT Notchset Feature Bento. No portal or audio
// dependencies are loaded by the public homepage. See /beautiful-ui-license.txt.
export { NOTCHSET_ROOT, OWN_SOUND } from "./root";
export function motionOff(el: Element | null | undefined) {
  if (typeof window === "undefined") return true;
  return Boolean(
    window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ||
    el?.closest('[data-motion="off"]'),
  );
}
export function RollingNumber({
  value,
  className,
}: {
  value: string | number;
  className?: string;
  height?: number;
}) {
  return <span className={className}>{value}</span>;
}
// The shipped Notchset root disables sound; preserve that policy explicitly.
export function playCue(_element: Element | null, _cue: string) {}
