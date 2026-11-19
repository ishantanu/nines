// Astryx fields take layout classes on the container and everything else on
// the control. Call sites pass one className, so split it by what it affects.
const LAYOUT =
  /^(?:[a-z]+:)*-?(?:m[trblxyse]?-|max-w-|min-w-|w-|col-|row-|self-|justify-self-|shrink|grow|basis-|order-|hidden$|block$|inline|sm:|md:|lg:)/;
export function splitFieldClasses(className?: string) {
  const container: string[] = [];
  const control: string[] = [];
  for (const token of (className ?? "").split(/\s+/).filter(Boolean))
    (LAYOUT.test(token) ? container : control).push(token);
  return {
    containerClassName: container.join(" ") || undefined,
    controlClassName: control.join(" ") || undefined,
  };
}
