import { stringify } from "yaml";

const object = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === "object" && !Array.isArray(value);
const format = (seconds: number) =>
  seconds !== 0 && seconds % 3600 === 0
    ? `${seconds / 3600}h`
    : seconds !== 0 && seconds % 60 === 0
      ? `${seconds / 60}m`
      : `${seconds}s`;

// Runtime API objects retain seconds; declarative YAML uses readable durations.
export function stringifyDefinition(value: unknown): string {
  return stringify(value, (_key, item: unknown) => {
    if (!object(item) || typeof item.burnRate !== "number") return item;
    const entries = Object.entries(item).map(([key, value]) => {
      const field =
        key === "shortWindowSeconds"
          ? "shortWindow"
          : key === "longWindowSeconds"
            ? "longWindow"
            : null;
      if (!field || typeof value !== "number" || !Number.isSafeInteger(value))
        return [key, value];
      if (field in item)
        throw Error(`Use either ${field} or ${field}Seconds, not both.`);
      return [field, format(value)];
    });
    return Object.fromEntries(entries);
  });
}

// Forms and evaluation readers use numeric seconds after strict parsing.
export function normalizeBurnWindows(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(normalizeBurnWindows);
  if (!object(value)) return value;
  const result = Object.fromEntries(
    Object.entries(value).map(([key, item]) => [
      key,
      normalizeBurnWindows(item),
    ]),
  );
  if (typeof result.burnRate !== "number") return result;
  for (const field of ["shortWindow", "longWindow"]) {
    if (!(field in result)) continue;
    if (`${field}Seconds` in result)
      throw Error(`Use either ${field} or ${field}Seconds, not both.`);
    const text = result[field];
    if (
      typeof text !== "string" ||
      text.length > 64 ||
      !/^([0-9]+[hms])+$/.test(text)
    )
      throw Error(`${field} requires a duration such as 300s, 5m or 1h30m.`);
    const seconds = [...text.matchAll(/([0-9]+)([hms])/g)].reduce(
      (sum, match) =>
        sum + Number(match[1]) * ({ h: 3600, m: 60, s: 1 }[match[2]] ?? 0),
      0,
    );
    if (!Number.isSafeInteger(seconds) || seconds <= 0 || seconds > 9223372036)
      throw Error(`${field} requires a positive whole-second duration.`);
    delete result[field];
    result[`${field}Seconds`] = seconds;
  }
  return result;
}
