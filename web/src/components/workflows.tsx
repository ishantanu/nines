import { useEffect, useState } from "react";
import { parse, stringify } from "yaml";
import { BookOpen, Bot, FileCheck2, Plus, RefreshCw } from "lucide-react";
import {
  api,
  date,
  message,
  workflowDecision,
  type Journey,
  type RequestScope,
  type Skill,
  type WorkflowRecord,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { cn } from "@/lib/utils";

function WorkflowCreate({
  scope,
  journeys,
  onCreated,
}: {
  scope: RequestScope;
  journeys: Journey[];
  onCreated: (id: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState("investigate");
  const [journeyId, setJourneyId] = useState("");
  const [manifest, setManifest] = useState("");
  const [note, setNote] = useState("");
  const [id, setID] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function create() {
    setBusy(true);
    setError("");
    try {
      const body =
        kind === "investigate"
          ? { id, kind, note, ...(journeyId ? { journeyId } : {}) }
          : { id, kind, note, manifest: parse(manifest) as unknown };
      const result = await api<WorkflowRecord>("/api/workflows", scope, {
        method: "POST",
        body: JSON.stringify(body),
      });
      setOpen(false);
      onCreated(result.id);
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog
      open={open}
      onOpenChange={(value) => {
        setOpen(value);
        if (value) {
          setID(`workflow-${Date.now()}`);
          setError("");
          setNote("");
          setManifest("");
          setJourneyId("");
          setKind("investigate");
        }
      }}
    >
      <DialogTrigger asChild>
        <Button size="sm">
          <Plus className="size-4" />
          New workflow
        </Button>
      </DialogTrigger>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>Start workflow</DialogTitle>
          <DialogDescription>
            Investigations inspect this tenant. Journey proposals capture a plan
            and wait for explicit approval before applying.
          </DialogDescription>
        </DialogHeader>
        <Label htmlFor="workflow-id">Workflow ID</Label>
        <Input
          id="workflow-id"
          value={id}
          onChange={(e) => setID(e.target.value)}
        />
        <Label htmlFor="workflow-kind">Workflow type</Label>
        <select
          id="workflow-kind"
          className="h-9 rounded-md border bg-white px-3 text-sm"
          value={kind}
          onChange={(e) => setKind(e.target.value)}
        >
          <option value="investigate">Investigate reliability</option>
          <option value="propose_journey">Propose journey change</option>
        </select>
        <Label htmlFor="workflow-journey">
          {kind === "investigate"
            ? "Investigation scope"
            : "Use an existing journey as a starting point"}
        </Label>
        <select
          id="workflow-journey"
          className="h-9 rounded-md border bg-white px-3 text-sm"
          value={journeyId}
          onChange={(e) => {
            setJourneyId(e.target.value);
            const selected = journeys.find(
              (j) => j.resource.manifest.metadata.name === e.target.value,
            );
            if (selected) setManifest(stringify(selected.resource.manifest));
          }}
        >
          <option value="">
            {kind === "investigate"
              ? "Entire tenant workspace"
              : "Choose a journey or paste a new manifest"}
          </option>
          {journeys.map((j) => (
            <option
              key={j.resource.manifest.metadata.name}
              value={j.resource.manifest.metadata.name}
            >
              {j.resource.manifest.spec.displayName}
            </option>
          ))}
        </select>
        {kind === "propose_journey" ? (
          <>
            <Label htmlFor="workflow-manifest">
              Proposed journey manifest (YAML)
            </Label>
            <Textarea
              id="workflow-manifest"
              className="min-h-64 font-mono text-xs"
              value={manifest}
              onChange={(e) => setManifest(e.target.value)}
              spellCheck={false}
            />
          </>
        ) : null}
        <Label htmlFor="workflow-note">Context for reviewers</Label>
        <Textarea
          id="workflow-note"
          value={note}
          onChange={(e) => setNote(e.target.value)}
          placeholder="Explain the reliability question or proposed change."
        />
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        <div className="dialog-actions">
          <Button
            disabled={
              busy ||
              !id.trim() ||
              (kind === "propose_journey" && !manifest.trim())
            }
            onClick={create}
          >
            {busy
              ? "Running…"
              : kind === "investigate"
                ? "Start investigation"
                : "Submit proposal"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

export function WorkflowDetail({
  workflow,
  scope,
  onChanged,
}: {
  workflow: WorkflowRecord;
  scope: RequestScope;
  onChanged: () => void;
}) {
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function decide(action: "approve" | "reject") {
    setBusy(true);
    setError("");
    try {
      await api(
        `/api/workflows/${encodeURIComponent(workflow.id)}/${action}`,
        scope,
        {
          method: "POST",
          body: JSON.stringify(workflowDecision(workflow, action, reason)),
        },
      );
      setReason("");
      onChanged();
    } catch (e) {
      setError(message(e));
      onChanged();
    } finally {
      setBusy(false);
    }
  }
  const actionable =
    scope.role === "owner" &&
    (workflow.state === "awaiting_approval" || workflow.state === "approved");
  return (
    <Card className="gap-0 py-0">
      <CardContent className="space-y-5 p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="break-all text-lg font-semibold">{workflow.id}</h2>
            <p className="mt-1 text-xs text-muted-foreground">
              {workflow.kind === "investigate"
                ? "Reliability investigation"
                : "Journey change proposal"}{" "}
              · version {workflow.version} · {date(workflow.updatedAt)}
            </p>
          </div>
          <Badge variant="outline">{workflow.state.replaceAll("_", " ")}</Badge>
        </div>
        {workflow.note ? (
          <p className="whitespace-pre-wrap text-sm text-muted-foreground">
            {workflow.note}
          </p>
        ) : null}
        <p className="break-all text-xs text-muted-foreground">
          Skill {workflow.skill.id} · {workflow.skill.version}
          <br />
          SHA-256 {workflow.skill.digest}
        </p>
        <div>
          <h3 className="mb-3 text-sm font-semibold">Findings</h3>
          {workflow.findings?.length ? (
            <ul className="space-y-2">
              {workflow.findings.map((finding, index) => (
                <li
                  key={`${finding.code}-${index}`}
                  className="rounded-md border p-3"
                >
                  <div className="mb-1 flex flex-wrap items-center gap-2">
                    <Badge
                      variant={
                        finding.severity === "critical"
                          ? "destructive"
                          : "secondary"
                      }
                    >
                      {finding.severity}
                    </Badge>
                    <span className="font-mono text-xs text-muted-foreground">
                      {finding.resource}
                    </span>
                  </div>
                  <p className="text-sm">{finding.message}</p>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-muted-foreground">
              No findings recorded.
            </p>
          )}
        </div>
        {workflow.proposal ? (
          <div className="space-y-3 rounded-md border bg-slate-50 p-4">
            <h3 className="flex items-center gap-2 text-sm font-semibold">
              <FileCheck2 className="size-4" />
              Review proposed revision
            </h3>
            <p className="text-xs text-muted-foreground">
              {workflow.proposal.manifest.metadata.name} ·{" "}
              {workflow.proposal.action} · current revision{" "}
              {workflow.proposal.expectedRevision}
            </p>
            <p className="break-all font-mono text-xs">
              SHA-256 {workflow.proposal.digest}
            </p>
            {workflow.proposal.previous ? (
              <details>
                <summary className="cursor-pointer text-xs">
                  Previous manifest
                </summary>
                <pre className="code-block mt-2">
                  {stringify(workflow.proposal.previous.manifest)}
                </pre>
              </details>
            ) : null}
            <details open>
              <summary className="cursor-pointer text-xs">
                Proposed manifest
              </summary>
              <pre className="code-block mt-2">
                {stringify(workflow.proposal.manifest)}
              </pre>
            </details>
          </div>
        ) : null}
        {actionable && workflow.proposal ? (
          <div className="space-y-3 border-t pt-4">
            <p className="text-xs text-muted-foreground">
              Approval applies this exact digest to tenant{" "}
              <strong>{scope.tenant}</strong>. A changed journey revision causes
              a conflict.
            </p>
            {workflow.state === "awaiting_approval" ? (
              <>
                <Label htmlFor="decision-reason">
                  Decision reason (required for rejection)
                </Label>
                <Textarea
                  id="decision-reason"
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                />
              </>
            ) : (
              <p className="text-xs text-muted-foreground">
                Resuming preserves the recorded approval decision and reason.
              </p>
            )}
            <div className="flex flex-wrap gap-2">
              <Button disabled={busy} onClick={() => decide("approve")}>
                {busy
                  ? "Working…"
                  : workflow.state === "approved"
                    ? "Resume approved apply"
                    : "Approve & apply revision"}
              </Button>
              {workflow.state === "awaiting_approval" ? (
                <Button
                  variant="outline"
                  disabled={busy || !reason.trim()}
                  onClick={() => decide("reject")}
                >
                  Reject proposal
                </Button>
              ) : null}
            </div>
          </div>
        ) : null}
        {workflow.applied ? (
          <p role="status" className="text-sm text-emerald-700">
            Journey revision {workflow.applied.revision} applied.
          </p>
        ) : null}
        {workflow.decision ? (
          <p className="whitespace-pre-wrap text-xs text-muted-foreground">
            {workflow.decision.action} ·{" "}
            {workflow.decision.reason || "No reason supplied"} ·{" "}
            {date(workflow.decision.at)}
          </p>
        ) : null}
        {workflow.error || error ? (
          <p role="alert" className="text-sm text-destructive">
            {error || workflow.error}
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}

export function Workflows({
  scope,
  journeys,
}: {
  scope: RequestScope;
  journeys: Journey[];
}) {
  const [workflows, setWorkflows] = useState<WorkflowRecord[]>([]);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [selected, setSelected] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    Promise.all([
      api<WorkflowRecord[]>("/api/workflows", scope, {
        signal: controller.signal,
      }),
      api<Skill[]>("/api/skills", scope, { signal: controller.signal }),
    ])
      .then(([runs, bundles]) => {
        if (!controller.signal.aborted) {
          setWorkflows(runs);
          setSkills(bundles);
          setError("");
        }
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted && !scope.signal.aborted)
          setError(message(e));
      })
      .finally(() => {
        if (!controller.signal.aborted && !scope.signal.aborted)
          setLoading(false);
      });
    return () => controller.abort();
  }, [scope, refresh]);
  const chosen = workflows.find((w) => w.id === selected) ?? workflows[0];
  return (
    <Tabs defaultValue="workflows">
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <TabsList>
          <TabsTrigger value="workflows">
            <Bot className="mr-1 size-4" />
            Workflows
          </TabsTrigger>
          <TabsTrigger value="skills">
            <BookOpen className="mr-1 size-4" />
            Skills
          </TabsTrigger>
        </TabsList>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setRefresh((n) => n + 1)}
            aria-label="Refresh workflows and skills"
          >
            <RefreshCw className={cn("size-4", loading && "animate-spin")} />
          </Button>
          {scope.role !== "viewer" ? (
            <WorkflowCreate
              scope={scope}
              journeys={journeys}
              onCreated={(id) => {
                setSelected(id);
                setRefresh((n) => n + 1);
              }}
            />
          ) : null}
        </div>
      </div>
      {error ? (
        <p
          role="alert"
          className="mb-4 rounded-md border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900"
        >
          {error}
        </p>
      ) : null}
      <TabsContent value="workflows">
        {loading && !workflows.length ? (
          <p role="status" className="empty-state">
            Loading tenant workflows…
          </p>
        ) : workflows.length ? (
          <div className="resource-split">
            <div className="space-y-2">
              {workflows.map((w) => (
                <button
                  key={w.id}
                  className={cn(
                    "journey-option w-full",
                    chosen?.id === w.id && "journey-selected",
                  )}
                  onClick={() => setSelected(w.id)}
                >
                  <span className="block truncate text-sm font-medium">
                    {w.id}
                  </span>
                  <span className="mt-2 block text-xs text-muted-foreground">
                    {w.state.replaceAll("_", " ")} · {date(w.createdAt)}
                  </span>
                </button>
              ))}
            </div>
            {chosen ? (
              <WorkflowDetail
                key={`${chosen.id}-${chosen.version}`}
                workflow={chosen}
                scope={scope}
                onChanged={() => setRefresh((n) => n + 1)}
              />
            ) : null}
          </div>
        ) : (
          <Card>
            <CardContent className="empty-state">
              <Bot className="mx-auto mb-3 size-7 text-muted-foreground" />
              <h2 className="font-semibold">No workflows in this tenant</h2>
              <p className="mt-2 text-sm">
                Run an investigation or propose a journey change for review.
              </p>
            </CardContent>
          </Card>
        )}
      </TabsContent>
      <TabsContent value="skills">
        <div className="panel-grid">
          {skills.map((skill) => (
            <Card key={skill.id}>
              <CardContent className="space-y-3 pt-6">
                <div className="flex items-center justify-between gap-3">
                  <h2 className="font-semibold">{skill.name}</h2>
                  <Badge variant="secondary">{skill.version}</Badge>
                </div>
                <p className="text-sm text-muted-foreground">
                  {skill.description}
                </p>
                <p className="break-all font-mono text-xs text-muted-foreground">
                  {skill.id}
                  <br />
                  SHA-256 {skill.digest}
                </p>
                <details>
                  <summary className="cursor-pointer text-sm font-medium">
                    Read skill instructions
                  </summary>
                  <pre className="code-block mt-3 whitespace-pre-wrap">
                    {skill.instructions}
                  </pre>
                </details>
              </CardContent>
            </Card>
          ))}
        </div>
        {!loading && !skills.length ? (
          <p className="empty-state">No skills are available.</p>
        ) : null}
        <p className="mt-4 text-xs text-muted-foreground">
          These versioned skills drive the built-in investigation and proposal
          workflows. Instructions are readable; changes require the proposal
          approval step.
        </p>
      </TabsContent>
    </Tabs>
  );
}
