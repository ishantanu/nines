// Adapted chart-only helpers for MIT Beautiful UI Notchset v1.0.0.
// See /beautiful-ui-license.txt. No audio, microphone or portal helpers are included.
export const NOTCHSET_ROOT = { "data-notchset": "" } as const;
export const GLIDE = "cubic-bezier(.65,0,.35,1)";
export function RollingLabel({
  text,
  className,
}: {
  text: string;
  className?: string;
  height?: number;
  delay?: number;
}) {
  return <span className={className}>{text}</span>;
}
