// Label sets are measurement data, not an authoring format.
export function MeasurementLabels({
  labels,
  empty = "No label filters",
}: {
  labels?: Record<string, string>;
  empty?: string;
}) {
  const entries = Object.entries(labels ?? {}).sort(([a], [b]) =>
    a.localeCompare(b),
  );
  if (!entries.length)
    return <span className="text-muted-foreground">{empty}</span>;
  return (
    <dl className="grid grid-cols-[minmax(0,auto)_minmax(0,1fr)] gap-x-4 gap-y-1 text-xs">
      {entries.map(([key, value]) => (
        <div key={key} className="contents">
          <dt className="break-all text-muted-foreground">{key}</dt>
          <dd className="break-all">{value || "Empty value"}</dd>
        </div>
      ))}
    </dl>
  );
}
