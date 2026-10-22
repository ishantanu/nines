import { useEffect, useRef, useState } from "react";
import { Play, RefreshCw } from "lucide-react";
import { Textarea } from "@/components/ui/textarea";
import { streamReviewRun } from "@/lib/review-stream";
import {
  reviewCapabilities,
  listReviewSkills,
  type ReviewSkill,
  type ReviewSkillRef,
  type ReviewCapabilities,
} from "@/lib/investigations";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { APIError, message, type RequestScope } from "@/lib/api";
import {
  cancelReviewRun,
  createReviewRun,
  listReviewRuns,
  readReviewRun,
  type Investigation,
  type ReviewRun,
} from "@/lib/investigations";
const active = (run: ReviewRun) =>
  run.status === "queued" || run.status === "running";
const readable = (value: string) => value.replaceAll("_", " ");
const errors: Record<string, string> = {
  skill_unavailable:
    "The pinned skill version is unavailable. Start a new run with an available version.",
  skill_input_invalid:
    "The run input does not match its pinned skill contract.",
  skill_evidence_missing:
    "The skill did not complete its required evidence reads.",
  skill_output_invalid: "The result does not match the skill output contract.",
  model_budget_exceeded:
    "The model review reached its input, output or tool budget.",
  model_configuration_changed:
    "The configured provider, model or review policy changed. Start a new run.",
  model_not_configured: "No review model is configured for this tenant.",
  model_capability_unavailable:
    "The endpoint did not support the required tool-calling response.",
  model_unavailable: "The configured model is unavailable.",
  model_refused: "The model declined this request.",
  model_truncated: "The model response exceeded its output-token limit.",
  model_incomplete: "The model response ended before completion.",
  model_invalid_response: "The model returned an invalid response.",
  unsafe_tool_call:
    "The model requested a tool or arguments outside this review's read-only scope.",
  invalid_model_citation:
    "The model did not cite the captured evidence correctly.",
  scope_unavailable:
    "The pinned cutoff is outside the available evidence or its context is invalid. Start a new review with a current cutoff.",
  access_revoked:
    "The initiating identity no longer has permission to run this review.",
  scope_revision_conflict:
    "The resource revision changed. Start a new review at its current revision.",
  resource_not_found: "The scoped resource is no longer available.",
  evidence_unavailable: "Evidence could not be read within the retry limit.",
  deadline_exceeded: "The run exceeded its five-minute deadline.",
  attempt_limit: "The run reached its recovery limit.",
};
export function ReviewRunResult({ run }: { run: ReviewRun }) {
  const assessment = run.steps.find((step) => step.assessment)?.assessment;
  const evidence = run.steps.find((step) => step.evidence)?.evidence;
  return (
    <div className="space-y-3 text-sm">
      <dl className="grid grid-cols-2 gap-3 text-xs">
        <div>
          <dt className="text-muted-foreground">Status</dt>
          <dd className="mt-1 font-medium capitalize">{run.status}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Requested by</dt>
          <dd className="mt-1 break-words">
            {run.requestedBy.name || "Workspace member"}
          </dd>
        </div>
      </dl>
      {run.skill ? (
        <details className="border-t pt-3 text-xs text-muted-foreground">
          <summary className="cursor-pointer">
            Skill {run.skill.id} · {run.skill.version}
          </summary>
          <p className="mt-2 break-all">SHA-256 {run.skill.digest}</p>
          {run.skillResult ? (
            <>
              <p className="mt-2">
                Evidence obligations and output schema verified.
              </p>
              <p className="mt-2 break-all">
                Output SHA-256 {run.skillResult.outputDigest}
              </p>
            </>
          ) : null}
        </details>
      ) : null}
      <ol className="space-y-2 border-y py-3 text-xs" aria-label="Run steps">
        {run.steps.map((step, index) => (
          <li
            key={`${index}:${step.tool}`}
            className="flex flex-wrap justify-between gap-2"
          >
            <span>
              {(
                {
                  read_context: step.callId
                    ? "Read captured context"
                    : "Capture scoped context",
                  assess_evidence: "Assess saved evidence",
                  model_turn: "Model request",
                  read_assessment: "Read calculated assessment",
                  read_objective: "Read captured objective",
                  finish_review: "Save model interpretation",
                } as Record<string, string>
              )[step.tool] ?? "Read-only step"}
            </span>
            <span className="text-muted-foreground">
              {readable(
                step.status === "running" &&
                  (run.status === "failed" || run.status === "cancelled")
                  ? run.status
                  : step.status,
              )}{" "}
              · attempts {step.attempts}
            </span>
          </li>
        ))}
      </ol>
      {run.errorCode ? (
        <p role="status" className="text-xs text-muted-foreground">
          {errors[run.errorCode] ?? "The run could not complete."}
          {active(run) ? " Retrying within the execution budget." : ""}
        </p>
      ) : null}
      {run.mode === "model" && run.question ? (
        <div className="space-y-1 border-t pt-3">
          <p className="text-xs font-medium">Question for this run</p>
          <p className="whitespace-pre-wrap break-words text-xs text-muted-foreground">
            {run.question}
          </p>
        </div>
      ) : null}
      {assessment ? (
        <div className="space-y-3">
          <p className="text-xs font-medium">
            {(
              {
                insufficient_evidence: "Insufficient evidence",
                window_within_target: "Window within target",
                window_below_target: "Window below target",
                linked_objectives_assessed: "Linked objectives assessed",
              } as Record<string, string>
            )[assessment.state] ?? readable(assessment.state)}
          </p>
          <ul className="space-y-3">
            {assessment.findings.map((finding, i) => (
              <li key={finding.resourceId + ":" + i} className="text-xs">
                <p className="font-medium break-words">{finding.resourceId}</p>
                <p className="mt-1 text-muted-foreground">{finding.message}</p>
              </li>
            ))}
          </ul>
          <details className="text-xs text-muted-foreground">
            <summary className="cursor-pointer">Assessment limits</summary>
            <ul className="mt-2 space-y-2">
              {assessment.limitations.map((limit, i) => (
                <li key={i}>{readable(limit)}</li>
              ))}
            </ul>
          </details>
        </div>
      ) : null}
      {run.mode === "model" ? (
        <div className="space-y-2 border-t pt-3 text-xs text-muted-foreground">
          <p>
            {run.modelRoute?.provider} · {run.modelRoute?.model}
          </p>
          <p>
            Model calls {run.modelCalls ?? 0}/4 · tool calls{" "}
            {run.toolCalls ?? 0}/4
          </p>
          {active(run) ? (
            <p role="status">
              Received {((run.receivedBytes ?? 0) / 1024).toFixed(1)} KiB of
              model response.
            </p>
          ) : null}
          <p>
            Output tokens reserved: {run.outputTokensReserved ?? 0}.{" "}
            {run.usageMissing
              ? "Provider usage is incomplete."
              : `Provider-reported output tokens: ${run.reportedOutputTokens ?? 0}.`}
          </p>
        </div>
      ) : null}
      {run.modelReview ? (
        <section
          className="space-y-3 border-t pt-3"
          aria-label="Model interpretation"
        >
          <h4 className="text-sm font-medium">Model interpretation</h4>
          <p className="text-xs text-muted-foreground">
            Model-generated text. The platform's calculated assessment remains
            above.
          </p>
          <p className="whitespace-pre-wrap break-words text-sm">
            {run.modelReview.summary}
          </p>
          {run.modelReview.limitations.length ? (
            <ul className="space-y-2 text-xs text-muted-foreground">
              {run.modelReview.limitations.map((value, i) => (
                <li key={i}>{value}</li>
              ))}
            </ul>
          ) : null}
          <details className="text-xs text-muted-foreground">
            <summary className="cursor-pointer">Evidence citations</summary>
            {run.modelReview.evidenceDigests.map((digest) => (
              <p key={digest} className="mt-2 break-all">
                {digest}
              </p>
            ))}
          </details>
        </section>
      ) : null}
      {evidence ? (
        <details className="border-t pt-3 text-xs text-muted-foreground">
          <summary className="cursor-pointer">Captured evidence</summary>
          <p className="mt-2 break-all">SHA-256: {evidence.digest}</p>
          <p className="mt-1">
            Captured {new Date(evidence.capturedAt).toLocaleString()}
          </p>
          <pre className="mt-3 max-h-64 overflow-auto whitespace-pre-wrap break-all rounded-md bg-muted p-3">
            {JSON.stringify(evidence.context, null, 2)}
          </pre>
        </details>
      ) : null}
      {run.status === "cancelled" ? (
        <p className="text-xs text-muted-foreground">
          Cancelled by {run.cancelledBy?.name || "an authorized member"}.
          Uncommitted results were discarded.
        </p>
      ) : null}
    </div>
  );
}
export function ReviewRuns({
  scope,
  review,
  writable,
  onConflict,
}: {
  scope: RequestScope;
  review: Investigation;
  writable: boolean;
  onConflict: () => void;
}) {
  const [capsError, setCapsError] = useState("");
  const [skills, setSkills] = useState<ReviewSkill[]>([]);
  const [skillError, setSkillError] = useState("");
  const [skillID, setSkillID] = useState("");
  const [caps, setCaps] = useState<ReviewCapabilities | null>(null);
  const [mode, setMode] = useState("deterministic");
  const [question, setQuestion] = useState(
    () => review.entries.filter((e) => e.kind === "message").at(-1)?.text ?? "",
  );
  const [streamEpoch, setStreamEpoch] = useState(0);
  const [runs, setRuns] = useState<ReviewRun[]>([]);
  const [selected, setSelected] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const mounted = useRef(true);
  const pending = useRef<{
    id: string;
    expectedVersion: number;
    mode?: string;
    question?: string;
    skill?: ReviewSkillRef;
  } | null>(null);
  const availableSkills = skills.filter(
    (s) =>
      s.spec.mode === mode && s.spec.scopeKinds.includes(review.scope.kind),
  );
  const chosenSkill =
    availableSkills.find(
      (s) => `${s.metadata.name}@${s.metadata.version}` === skillID,
    ) ??
    availableSkills.find(
      (s) =>
        s.metadata.name ===
        (mode === "model" ? "reliability-review" : "evidence-check"),
    );
  const running = runs.find(active);
  const pollingID = running?.id;
  const chosen = runs.find((run) => run.id === selected) ?? runs.at(-1);
  const accept = (run: ReviewRun) =>
    setRuns((prev) => {
      const old = prev.find((item) => item.id === run.id);
      if (old && old.version > run.version) return prev;
      return [...prev.filter((item) => item.id !== run.id), run].sort(
        (a, b) =>
          a.createdAt.localeCompare(b.createdAt) || a.id.localeCompare(b.id),
      );
    });
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    let current = true;
    listReviewRuns(scope, review.id)
      .then((list) => {
        if (current) setRuns(list.items);
      })
      .catch((e) => {
        if (current && !scope.signal.aborted) setError(message(e));
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
    };
  }, [scope, review.id]);
  useEffect(() => {
    let current = true;
    listReviewSkills(scope)
      .then((value) => {
        if (current) {
          setSkills(value.items);
          setSkillError("");
        }
      })
      .catch((e) => {
        if (current && !scope.signal.aborted) setSkillError(message(e));
      });
    return () => {
      current = false;
    };
  }, [scope, streamEpoch]);
  useEffect(() => {
    let current = true;
    reviewCapabilities(scope)
      .then((value) => {
        if (current) {
          setCaps(value);
          setCapsError("");
        }
      })
      .catch((error) => {
        if (current && !scope.signal.aborted) setCapsError(message(error));
      });
    return () => {
      current = false;
    };
  }, [scope, streamEpoch]);
  useEffect(() => {
    if (!pollingID) return;
    const controller = new AbortController();
    let current = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let version = 0;
    let failures = 0;
    const follow = async () => {
      try {
        const terminal = await streamReviewRun(
          scope,
          review.id,
          pollingID,
          version,
          controller.signal,
          (run) => {
            if (current) {
              version = run.version;
              accept(run);
              setError("");
            }
          },
        );
        failures = 0;
        if (terminal) return;
      } catch (e) {
        if (!current || scope.signal.aborted || controller.signal.aborted)
          return;
        setError(message(e));
        if (e instanceof APIError && (e.status === 401 || e.status === 403))
          return;
        if (++failures >= 3) return;
      }
      if (current)
        timer = setTimeout(() => void follow(), failures ? 1500 : 100);
    };
    void follow();
    return () => {
      current = false;
      controller.abort();
      if (timer) clearTimeout(timer);
    };
  }, [scope, review.id, pollingID, streamEpoch]);
  async function reload() {
    setBusy(true);
    setStreamEpoch((v) => v + 1);
    try {
      const list = await listReviewRuns(scope, review.id);
      if (mounted.current) {
        setRuns(list.items);
        setError("");
      }
    } catch (e) {
      if (mounted.current && !scope.signal.aborted) setError(message(e));
    } finally {
      if (mounted.current) setBusy(false);
    }
  }
  async function start() {
    setBusy(true);
    setError("");
    pending.current ??= {
      id: crypto.randomUUID(),
      expectedVersion: review.version,
      ...(mode === "model" ? { mode, question } : {}),
      ...(chosenSkill
        ? {
            skill: {
              id: chosenSkill.metadata.name,
              version: chosenSkill.metadata.version,
              digest: chosenSkill.digest,
            },
          }
        : {}),
    };
    try {
      const run = await createReviewRun(scope, review.id, pending.current);
      if (!mounted.current || scope.signal.aborted) return;
      pending.current = null;
      accept(run);
      setSelected(run.id);
    } catch (e) {
      if (!mounted.current || scope.signal.aborted) return;
      if (e instanceof APIError && [400, 403, 409].includes(e.status)) {
        pending.current = null;
        if (e.status === 409) onConflict();
      }
      setError(message(e));
    } finally {
      if (mounted.current) setBusy(false);
    }
  }
  async function cancel() {
    if (!chosen) return;
    setBusy(true);
    setError("");
    try {
      const run = await cancelReviewRun(
        scope,
        review.id,
        chosen.id,
        chosen.version,
      );
      if (mounted.current && !scope.signal.aborted) accept(run);
    } catch (e) {
      if (!mounted.current || scope.signal.aborted) return;
      setError(message(e));
      if (e instanceof APIError && e.status === 409) {
        try {
          const run = await readReviewRun(scope, review.id, chosen.id);
          if (mounted.current) accept(run);
        } catch {
          /* Preserve the original conflict. */
        }
      }
    } finally {
      if (mounted.current) setBusy(false);
    }
  }
  return (
    <section className="space-y-4 p-4" aria-label="Read-only runs">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-medium">Read-only runs</h3>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="icon"
            disabled={busy}
            onClick={() => void reload()}
            aria-label="Refresh runs"
          >
            <RefreshCw className="size-4" />
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={
              !writable ||
              busy ||
              loading ||
              !!running ||
              runs.length >= 20 ||
              !chosenSkill ||
              !!skillError ||
              (mode === "model" && (!caps?.model || !question.trim()))
            }
            onClick={() => void start()}
          >
            <Play className="size-3.5" />
            Run review
          </Button>
        </div>
      </div>
      <p className="text-xs text-muted-foreground">
        {mode === "model"
          ? "The selected model receives your question and captured evidence. Configuration remains read-only."
          : "Stored context only. No model calls or configuration changes."}
      </p>
      <div>
        <Label htmlFor="review-mode">Review mode</Label>
        <Select
          value={mode}
          onValueChange={(value) => {
            setMode(value);
            setSkillID("");
            if (value === "model" && !question.trim())
              setQuestion(
                review.entries
                  .filter((entry) => entry.kind === "message")
                  .at(-1)?.text ?? "",
              );
            pending.current = null;
          }}
          disabled={!writable || busy || !!running}
        >
          <SelectTrigger id="review-mode" className="mt-2">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="deterministic">Evidence check</SelectItem>
            <SelectItem value="model" disabled={!caps?.model}>
              Model review
            </SelectItem>
          </SelectContent>
        </Select>
        {!caps?.model ? (
          <p className="mt-2 text-xs text-muted-foreground">
            {capsError
              ? "Unable to read model configuration. Refresh runs to retry."
              : "No review model configured."}
          </p>
        ) : null}
      </div>
      <div className="space-y-2">
        <Label htmlFor="review-skill">Execution skill</Label>
        <Select
          value={
            chosenSkill
              ? `${chosenSkill.metadata.name}@${chosenSkill.metadata.version}`
              : ""
          }
          disabled={!writable || busy || !!running || !availableSkills.length}
          onValueChange={(value) => {
            setSkillID(value);
            pending.current = null;
          }}
        >
          <SelectTrigger id="review-skill">
            <SelectValue placeholder="Loading skills…" />
          </SelectTrigger>
          <SelectContent>
            {availableSkills.map((skill) => (
              <SelectItem
                key={skill.metadata.name + skill.metadata.version}
                value={`${skill.metadata.name}@${skill.metadata.version}`}
              >
                {skill.spec.displayName} · {skill.metadata.version}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {skillError ? (
          <p role="alert" className="text-xs text-destructive">
            Unable to load execution skills. Refresh runs to retry.
          </p>
        ) : null}
        {chosenSkill ? (
          <details className="text-xs text-muted-foreground">
            <summary className="cursor-pointer">Skill contract</summary>
            <p className="mt-2">{chosenSkill.spec.description}</p>
            <p className="mt-2">
              Allowed tools:{" "}
              {chosenSkill.spec.toolAllowlist.map(readable).join(", ")}
            </p>
            <p className="mt-2">
              Required tools:{" "}
              {chosenSkill.spec.requiredTools.length
                ? chosenSkill.spec.requiredTools.map(readable).join(", ")
                : "Captured context and calculated assessment"}
            </p>
            <p className="mt-2 break-all">SHA-256 {chosenSkill.digest}</p>
          </details>
        ) : null}
      </div>
      {mode === "model" ? (
        <div className="space-y-2">
          <Label htmlFor="model-review-question">Review question</Label>
          <Textarea
            id="model-review-question"
            rows={3}
            maxLength={4000}
            value={question}
            disabled={!writable || busy || !!running}
            onChange={(event) => {
              setQuestion(event.target.value);
              pending.current = null;
            }}
          />
          <p className="text-xs text-muted-foreground">
            {caps?.model?.provider} · {caps?.model?.model}
            {caps?.model?.local ? " · local endpoint" : ""}
          </p>
        </div>
      ) : null}
      {error ? (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      ) : null}
      {runs.length > 1 ? (
        <div>
          <Label htmlFor="review-run">Run history</Label>
          <Select value={chosen?.id} onValueChange={setSelected}>
            <SelectTrigger id="review-run" className="mt-2">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {[...runs].reverse().map((run) => (
                <SelectItem key={run.id} value={run.id}>
                  {new Date(run.createdAt).toLocaleString()} · {run.status}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      ) : null}
      {chosen ? (
        <>
          <ReviewRunResult run={chosen} />
          {active(chosen) && writable ? (
            <div className="flex justify-end">
              <Button
                variant="outline"
                size="sm"
                disabled={busy}
                title="Only the run initiator or an owner may cancel."
                onClick={() => void cancel()}
              >
                Cancel run
              </Button>
            </div>
          ) : null}
        </>
      ) : (
        <p className="text-xs text-muted-foreground">
          {loading ? "Loading runs…" : "No runs recorded."}
        </p>
      )}
    </section>
  );
}
