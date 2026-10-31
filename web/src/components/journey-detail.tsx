import { SLOProvider } from "./provider-identity";
import type { Workspace } from "@/lib/api";
import { VisualEvidence } from "./visual-evidence";
import { SLIValue } from "@/components/sli-value";
import { useEffect, useState } from "react";
import { stringifyDefinition as stringify } from "@/lib/definition-yaml";
import {
  Bot,
  GitCommitHorizontal,
  GitBranch,
  ShieldCheck,
  Timer,
  Coins,
  ExternalLink,
} from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  api,
  type RequestScope,
  date,
  message,
  percent,
  type Journey,
  type Revision,
  type Run,
} from "@/lib/api";
import { Status } from "./status";
import { ManifestEditor } from "./manifest-editor";
import { JourneyMap } from "./journey-map";
function RunRecorder({
  journey,
  scope,
  onSaved,
}: {
  journey: Journey;
  scope: RequestScope;
  onSaved: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function save() {
    setBusy(true);
    setError("");
    try {
      const evidence: unknown = JSON.parse(text);
      await api(
        `/api/journeys/${journey.resource.manifest.metadata.name}/runs`,
        scope,
        { method: "POST", body: JSON.stringify(evidence) },
      );
      setOpen(false);
      onSaved();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        setOpen(v);
        if (v) {
          const end = new Date();
          setText(
            JSON.stringify(
              {
                id: `run-${end.getTime()}`,
                journeyRevision: journey.resource.revision,
                startedAt: new Date(end.getTime() - 30000).toISOString(),
                finishedAt: end.toISOString(),
                outcome: "success",
                costUSD: 0.08,
                steps: 5,
                approvalObserved: true,
              },
              null,
              2,
            ),
          );
          setError("");
        }
      }}
    >
      <DialogTrigger asChild>
        <Button size="sm" variant="outline">
          Record evidence
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Record run evidence</DialogTitle>
          <DialogDescription>
            Submit observed evidence from your workflow. Verdicts are evaluated
            against the specified immutable contract revision.
          </DialogDescription>
        </DialogHeader>
        <Label htmlFor="run-evidence">Run evidence (JSON)</Label>
        <Textarea
          id="run-evidence"
          className="min-h-72 font-mono text-xs"
          spellCheck={false}
          value={text}
          onChange={(e) => setText(e.target.value)}
        />
        {error ? (
          <p role="alert" className="text-destructive text-sm">
            {error}
          </p>
        ) : null}
        <div className="dialog-actions">
          <Button disabled={busy} onClick={save}>
            {busy ? "Saving…" : "Record run"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
export function JourneyDetail({
  journey,
  connections,
  scope,
  onSLO,
  onRefresh,
  onProposed,
  onReplays,
  evidenceOnly = false,
  refreshKey = 0,
}: {
  journey: Journey;
  connections?: Workspace["connections"];
  scope: RequestScope;
  onSLO: (id: string) => void;
  onRefresh: () => void;
  onProposed?: () => void;
  onReplays?: () => void;
  evidenceOnly?: boolean;
  refreshKey?: number;
}) {
  const [runs, setRuns] = useState<Run[]>([]);
  const [history, setHistory] = useState<Revision[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [refresh, setRefresh] = useState(0);
  const resource = journey.resource;
  const m = resource.manifest;
  const a = m.spec.agentic;
  useEffect(() => {
    const controller = new AbortController();
    setError("");
    Promise.all([
      api<Run[]>(`/api/journeys/${m.metadata.name}/runs`, scope, {
        signal: controller.signal,
      }),
      api<Revision[]>(`/api/journeys/${m.metadata.name}/history`, scope, {
        signal: controller.signal,
      }),
    ])
      .then(([r, h]) => {
        if (!controller.signal.aborted) {
          setRuns(r);
          setHistory(h);
        }
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError(message(e));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [m.metadata.name, resource.revision, scope, refresh, refreshKey]);
  return (
    <Card className="gap-0 overflow-hidden py-0 shadow-none">
      <div className="border-b p-5">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Badge variant="outline" className="capitalize text-xs">
              {m.spec.criticality}
            </Badge>
            {a ? (
              <Badge variant="secondary">
                <Bot className="size-3" />
                Agentic
              </Badge>
            ) : null}
          </div>
          <Status value={journey.health} />
        </div>
        <h2 className="text-xl font-semibold tracking-tight">
          {m.spec.displayName}
        </h2>
        <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
          {m.spec.description}
        </p>
        <div className="mt-4 flex flex-wrap items-center gap-4 text-xs text-muted-foreground">
          <span>{m.spec.owner}</span>
          <span className="flex items-center gap-1">
            <GitBranch className="size-3" />
            {m.metadata.project}
          </span>
          <span className="flex items-center gap-1">
            <GitCommitHorizontal className="size-3" />
            Revision {resource.revision}
          </span>
          {onReplays ? (
            <Button variant="outline" size="sm" onClick={onReplays}>
              Session replay
            </Button>
          ) : null}
          <ManifestEditor
            scope={scope}
            resource={resource}
            onApplied={onRefresh}
            onProposed={onProposed}
          />
        </div>
      </div>
      <CardContent className="p-5">
        <Tabs
          defaultValue={evidenceOnly ? "evidence" : "overview"}
          key={`${m.metadata.name}-${evidenceOnly}`}
          className="detail-tabs"
        >
          <TabsList className="detail-tab-list mb-4">
            <TabsTrigger value="overview">Overview</TabsTrigger>
            <TabsTrigger value="manifest">Manifest</TabsTrigger>
            <TabsTrigger value="evidence">Run evidence</TabsTrigger>
            <TabsTrigger value="history">History</TabsTrigger>
          </TabsList>
          <TabsContent value="overview" className="space-y-6">
            <div>
              <div className="section-label mb-3">Journey map</div>
              <JourneyMap journey={journey} onSLO={onSLO} />
            </div>
            <div>
              <div className="section-label mb-2">Linked objectives</div>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Objective</TableHead>
                    <TableHead>Role</TableHead>
                    <TableHead>Budget remaining</TableHead>
                    <TableHead>SLI / SLO target</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {journey.links.map((l) => (
                    <TableRow key={l.link.id}>
                      <TableCell>
                        <button
                          className="text-left text-sm font-medium hover:text-primary"
                          onClick={() => onSLO(l.link.id)}
                        >
                          {l.slo.name}
                          <ExternalLink className="ml-1 inline size-3" />
                        </button>
                        <div className="mt-1">
                          <SLOProvider slo={l.slo} connections={connections} />
                        </div>
                        <div className="text-xs text-muted-foreground">
                          {l.link.dimension}
                        </div>
                      </TableCell>
                      <TableCell className="text-xs">
                        {l.link.required ? "Required" : "Advisory"}
                      </TableCell>
                      <TableCell className="font-mono text-xs">
                        {percent(l.status.budgetRemainingFraction, 1)}
                      </TableCell>
                      <TableCell>
                        <SLIValue status={l.status} target={l.slo.target} />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              <p className="mt-3 text-xs leading-relaxed text-muted-foreground">
                Health reflects required objectives. Partial coverage and
                missing traffic remain unknown. An end-to-end success rate
                requires a separately instrumented SLI.
              </p>
            </div>
            {a ? (
              <div className="rounded-md border bg-slate-50 p-4">
                <div className="mb-2 flex items-center gap-2 text-sm font-semibold">
                  <Bot className="size-4 text-slate-500" />
                  Agent contract
                </div>
                <p className="text-sm text-slate-600">{a.successDefinition}</p>
                <div className="mt-4 flex flex-wrap gap-4 text-xs text-slate-600">
                  <span className="flex items-center gap-1">
                    <Timer className="size-3.5" />
                    {a.maxDurationSeconds ?? "Unbounded"}s
                  </span>
                  <span className="flex items-center gap-1">
                    <Coins className="size-3.5" />
                    {a.maxCostUSD == null
                      ? "No cost limit"
                      : `$${a.maxCostUSD.toFixed(2)} / run`}
                  </span>
                  <span>{a.maxSteps ?? "Unbounded"} steps</span>
                  <span className="flex items-center gap-1">
                    <ShieldCheck className="size-3.5" />
                    {a.approvalRequired
                      ? "Approval evidence required"
                      : "Approval optional"}
                  </span>
                </div>
              </div>
            ) : null}
          </TabsContent>
          <TabsContent value="manifest">
            <div className="mb-3 flex items-center justify-between">
              <span className="section-label">
                {m.metadata.name}.journey.yaml
              </span>
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  const blob = new Blob([stringify(m)], {
                    type: "application/yaml",
                  });
                  const url = URL.createObjectURL(blob);
                  const link = document.createElement("a");
                  link.href = url;
                  link.download = `${m.metadata.name}.journey.yaml`;
                  link.click();
                  setTimeout(() => URL.revokeObjectURL(url), 1000);
                }}
              >
                Download YAML
              </Button>
            </div>
            <pre className="code-block">{stringify(m)}</pre>
            <p className="mt-3 break-all font-mono text-xs text-muted-foreground">
              SHA-256 {resource.digest}
            </p>
          </TabsContent>
          <TabsContent value="evidence">
            <VisualEvidence
              key={`${scope.tenant}:${resource.manifest.metadata.name}:${resource.revision}`}
              scope={scope}
              journeyID={resource.manifest.metadata.name}
              revision={resource.revision}
            />
            {a ? (
              <>
                <div className="mb-4 flex items-center justify-between gap-2">
                  <p className="text-xs text-muted-foreground">
                    Latest 200 ingested runs, across contract revisions.
                  </p>
                  {scope.role !== "viewer" ? (
                    <RunRecorder
                      journey={journey}
                      scope={scope}
                      onSaved={() => setRefresh((n) => n + 1)}
                    />
                  ) : null}
                </div>
                {loading ? (
                  <p className="empty-state">Loading evidence…</p>
                ) : runs.length ? (
                  <>
                    <div className="mb-4 grid grid-cols-3 gap-3">
                      {["passed", "violated", "incomplete"].map((v) => (
                        <div className="rounded-md border p-3" key={v}>
                          <div className="text-2xl font-semibold">
                            {runs.filter((r) => r.verdict === v).length}
                          </div>
                          <div className="text-xs capitalize text-muted-foreground">
                            {v}
                          </div>
                        </div>
                      ))}
                    </div>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>Run / revision</TableHead>
                          <TableHead>Outcome</TableHead>
                          <TableHead>Cost</TableHead>
                          <TableHead>Contract</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {runs.map((r) => (
                          <TableRow key={r.evidence.id}>
                            <TableCell className="max-w-52">
                              <div className="truncate font-mono text-xs">
                                {r.evidence.id} · v{r.evidence.journeyRevision}
                              </div>
                              <div className="mt-1 text-xs text-muted-foreground">
                                {date(r.evidence.finishedAt)}
                              </div>
                              {r.findings.length ? (
                                <div className="mt-1 text-xs text-rose-700">
                                  {r.findings.join(" · ")}
                                </div>
                              ) : null}
                            </TableCell>
                            <TableCell className="text-xs capitalize">
                              {r.evidence.outcome}
                            </TableCell>
                            <TableCell className="font-mono text-xs">
                              {r.evidence.costUSD == null
                                ? "Unknown"
                                : `$${r.evidence.costUSD.toFixed(3)}`}
                            </TableCell>
                            <TableCell>
                              <Status value={r.verdict} />
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </>
                ) : (
                  <p className="empty-state">
                    No run evidence yet. Instrument your workflow to POST
                    observations to this journey’s runs endpoint.
                  </p>
                )}
                <p className="mt-4 text-xs leading-relaxed text-muted-foreground">
                  Outcomes and approvals are reported observations. Contract
                  checks do not execute workflows or enforce approvals. Run
                  evidence remains separate from linked SLO metrics.
                </p>
              </>
            ) : (
              <p className="empty-state">
                Add an agentic contract to this journey to evaluate workflow
                evidence.
              </p>
            )}
          </TabsContent>
          <TabsContent value="history">
            {loading ? (
              <p className="empty-state">Loading revisions…</p>
            ) : (
              history.map((r) => (
                <details key={r.revision} className="border-b py-4">
                  <summary className="cursor-pointer text-sm">
                    <span className="font-semibold">Revision {r.revision}</span>
                    <span className="ml-3 text-xs text-muted-foreground">
                      {date(r.appliedAt)} · {r.digest.slice(0, 12)}
                    </span>
                  </summary>
                  <pre className="code-block mt-4">{stringify(r.manifest)}</pre>
                </details>
              ))
            )}
          </TabsContent>
        </Tabs>
        {error ? (
          <p role="alert" className="mt-4 text-sm text-destructive">
            {error}
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}
