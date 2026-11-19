import { useEffect, useRef, useState } from "react";
import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
import {
  api,
  message,
  percent,
  type CustomerReport,
  type RequestScope,
} from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

const stateText: Record<string, string> = {
  observed: "Observed",
  low_traffic: "Low traffic",
  no_traffic: "No traffic",
  missing_numerator: "Numerator unknown",
  no_series: "No data",
};

// An explicit, bounded provider read, like cohort inspection. A customer
// without data is shown as "No data", never as healthy.
export function CustomerReportPanel({
  sloId,
  revision,
  scope,
}: {
  sloId: string;
  revision: number;
  scope: RequestScope;
}) {
  const [report, setReport] = useState<CustomerReport>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const request = useRef(new AbortController());
  useEffect(() => {
    request.current.abort();
    request.current = new AbortController();
    setReport(undefined);
    setBusy(false);
    setError("");
    return () => request.current.abort();
  }, [sloId, revision, scope]);
  async function inspect() {
    const controller = request.current;
    setBusy(true);
    setError("");
    try {
      const next = await api<CustomerReport>(
        `/api/slos/${encodeURIComponent(sloId)}/customers?expectedRevision=${revision}`,
        scope,
        { signal: controller.signal },
      );
      if (!controller.signal.aborted) setReport(next);
    } catch (e) {
      if (!controller.signal.aborted) setError(message(e));
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  return (
    <Card className="gap-0 py-0" aria-label="Per-customer reliability">
      <CardHeader className="border-b py-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <Heading level={2}>Per-customer reliability</Heading>
            <Text as="p" size="sm" color="secondary">
              The same SLI for a short, explicit list of customers. An average
              can hide a customer who is failing.
            </Text>
          </div>
          <Button variant="outline" size="sm" disabled={busy} onClick={inspect}>
            {busy ? "Inspecting customers…" : "Inspect customers"}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3 py-4">
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        {report ? (
          <>
            <Text as="p" size="sm" role="status">
              {report.message}
            </Text>
            {report.state === "available" ? (
              <>
                <Text as="p" size="sm" color="secondary">
                  Service-wide burn{" "}
                  {report.serviceBurnRate === null
                    ? "unknown"
                    : `${report.serviceBurnRate.toFixed(2)}×`}
                  . Cutoff {new Date(report.at).toISOString()}.
                </Text>
                <Table aria-label="Reliability by customer">
                  <TableHeader>
                    <TableRow>
                      <TableHead>Customer</TableHead>
                      <TableHead>SLI</TableHead>
                      <TableHead>Target</TableHead>
                      <TableHead>Burn</TableHead>
                      <TableHead>Events</TableHead>
                      <TableHead>Evidence</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {report.items.map((c) => (
                      <TableRow key={c.alias}>
                        <TableCell className="font-medium">{c.name}</TableCell>
                        <TableCell className="tabular-nums">
                          {c.reliability === null
                            ? "Unavailable"
                            : percent(c.reliability, 3)}
                        </TableCell>
                        <TableCell className="tabular-nums">
                          {percent(c.target, 3)}
                        </TableCell>
                        <TableCell className="tabular-nums">
                          {c.burnRate === null
                            ? "—"
                            : `${c.burnRate.toFixed(2)}×`}
                        </TableCell>
                        <TableCell className="tabular-nums">
                          {c.total.toLocaleString()}
                        </TableCell>
                        <TableCell>
                          {c.maskedByAggregate ? (
                            <Badge variant="destructive">
                              Hidden by the average
                            </Badge>
                          ) : (
                            <span>{stateText[c.state] ?? c.state}</span>
                          )}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </>
            ) : null}
            <ul className="list-disc space-y-1 pl-5">
              {report.limitations.map((text) => (
                <li key={text}>
                  <Text as="span" size="sm" color="secondary">
                    {text}
                  </Text>
                </li>
              ))}
            </ul>
          </>
        ) : null}
      </CardContent>
    </Card>
  );
}
