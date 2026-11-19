// MIT Beautiful UI Notchset v1.0.0; see /beautiful-ui-license.txt.
import type * as React from "react";

/**
 * A chart's numbers as a table for screen readers (visually hidden). Charts draw their data for the
 * eye and show values on hover; this gives keyboard and assistive-tech users every value without
 * either. Server-safe.
 */
export function ChartTable({
  caption,
  head,
  rows,
}: {
  caption: React.ReactNode;
  head: readonly React.ReactNode[];
  rows: readonly (readonly React.ReactNode[])[];
}) {
  return (
    <table className="sr-only select-none">
      <caption>{caption}</caption>
      <thead>
        <tr>
          {head.map((h, i) => (
            <th key={i} scope="col">
              {h}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((r, i) => (
          <tr key={i}>
            {r.map((c, j) =>
              j === 0 ? (
                <th key={j} scope="row">
                  {c}
                </th>
              ) : (
                <td key={j}>{c}</td>
              ),
            )}
          </tr>
        ))}
      </tbody>
    </table>
  );
}
