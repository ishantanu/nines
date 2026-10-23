import { useResourceRoute } from "@/lib/navigation";
import { useEffect, useRef, useState } from "react";
import { stringify } from "yaml";
import {
  api,
  canEdit,
  message,
  type Journey,
  type RequestScope,
} from "@/lib/api";
import {
  intentPopulation,
  backtestTime,
  type IntentDocument,
} from "@/lib/intent";
import {
  authoringScope,
  conversation,
  conversations,
  conversationBase,
  type IntentConversation,
  type ConversationSummary,
  type FlowCapabilities,
  type IntentVerification,
} from "@/lib/intent-conversations";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Select,
  SelectTrigger,
  SelectContent,
  SelectItem,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { IntentMeasurementEditor } from "./intent-measurement";
import { IntentAuthoring } from "./intent-authoring";
import { IntentEvidence, IntentProgress } from "./intent-evidence";
import {
  IntentRecommendationReport,
  IntentDecisionControls,
  IntentGovernanceHistory,
} from "./intent-governance";
import { duration } from "@/lib/history";
export function IntentWorkspace({
  scope,
  journeys,
  onApplied,
}: {
  scope: RequestScope;
  journeys: Journey[];
  onApplied: () => void;
}) {
  const [kubeNamespace, setKubeNamespace] = useState("");
  const [verification, setVerification] = useState<IntentVerification | null>(
    null,
  );
  const [tab, setTab] = useState("conversations");
  const mutation = useRef(false);
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, [scope]);
  const [items, setItems] = useState<ConversationSummary[]>([]),
    [cursor, setCursor] = useState("");
  const [selected, setSelected] = useResourceRoute(scope.tenant, "intents");
  const [pollEpoch, setPollEpoch] = useState(0);
  const messagesRef = useRef<HTMLDivElement>(null);
  const followMessages = useRef(true);
  const activeID = useRef(selected);
  activeID.current = selected;
  const [storedValue, setValue] = useState<IntentConversation | null>(null),
    [caps, setCaps] = useState<FlowCapabilities | null>(null);
  const value = storedValue?.id === selected ? storedValue : null;
  const activeChangeDigest = useRef<string | undefined>(undefined);
  activeChangeDigest.current = value?.changeSet?.digest;
  const [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(false);
  const [createOpen, setCreateOpen] = useState(false),
    [reply, setReply] = useState("");
  const [mode, setMode] = useState("guided"),
    [consent, setConsent] = useState(false),
    [ack, setAck] = useState(false);
  const [history, setHistory] = useState(false),
    [from, setFrom] = useState(""),
    [to, setTo] = useState(""),
    [resolution, setResolution] = useState("300");
  const [draft, setDraft] = useState({
    name: "",
    project: "",
    service: "",
    source: "",
    population: "",
    text: "",
  });
  const writable = scope.role !== "viewer";
  const run = value?.runs.at(-1);
  const running = run?.status === "queued" || run?.status === "running";
  const sharesModel =
    mode === "model" || (!value?.goal && !!caps?.draftEnabled);
  useEffect(() => {
    setVerification(null);
  }, [selected, value?.changeSet?.digest, value?.changeSet?.state]);
  async function exportKubernetes() {
    if (!value?.changeSet) return;
    const digest = value.changeSet.digest;
    const r = await api<{
      tenant: string;
      namespace: string;
      changeSetDigest: string;
      yaml: string;
    }>(
      conversationBase +
        "/" +
        encodeURIComponent(value.id) +
        "/change-set/kubernetes?namespace=" +
        encodeURIComponent(kubeNamespace) +
        "&digest=" +
        digest,
      scope,
    );
    if (
      r.tenant !== scope.tenant ||
      r.namespace !== kubeNamespace ||
      r.changeSetDigest !== digest
    )
      throw Error("Kubernetes export scope mismatch");
    if (
      !scope.signal.aborted &&
      activeID.current === value.id &&
      activeChangeDigest.current === digest
    )
      exportYAML("nines-" + value.id + ".kubernetes.yaml", r.yaml);
  }
  async function verifyChanges() {
    if (!value?.changeSet) return;
    const expected = value.changeSet.digest;
    const r = await api<IntentVerification>(
      conversationBase +
        "/" +
        encodeURIComponent(value.id) +
        "/change-set/verification",
      scope,
    );
    if (
      r.tenant !== scope.tenant ||
      r.conversation !== value.id ||
      r.digest !== expected
    )
      throw Error("Change-set verification scope mismatch");
    if (
      !scope.signal.aborted &&
      activeID.current === value.id &&
      activeChangeDigest.current === expected
    )
      setVerification(r);
  }
  async function refreshList(next = "") {
    const r = await conversations(scope, next);
    if (scope.signal.aborted) return;
    setItems((prev) =>
      next
        ? [...prev, ...r.items.filter((x) => !prev.some((p) => p.id === x.id))]
        : r.items,
    );
    setCursor(r.nextCursor ?? "");
  }
  useEffect(() => {
    let alive = true;
    void Promise.all([
      conversations(scope),
      api<FlowCapabilities>(conversationBase + "/capabilities", scope),
    ])
      .then(([list, c]) => {
        if (alive) {
          setItems(list.items);
          setCursor(list.nextCursor ?? "");
          setCaps(c);
          setMode(c.modelEnabled ? "model" : "guided");
        }
      })
      .catch((e) => {
        if (alive && !scope.signal.aborted) setError(message(e));
      });
    return () => {
      alive = false;
    };
  }, [scope]);
  useEffect(() => {
    setReply("");
    setConsent(false);
    setAck(false);
    followMessages.current = true;
  }, [selected]);
  useEffect(() => {
    if (!selected) {
      setValue(null);
      return;
    }
    let alive = true;
    let timer: ReturnType<typeof setTimeout>;
    setLoading(true);
    setError("");
    setAck(false);
    async function load() {
      try {
        const r = await conversation(scope, selected);
        if (!alive) return;
        setValue((prev) =>
          prev?.id === r.id && prev.version > r.version ? prev : r,
        );
        setLoading(false);
        const latest = r.runs.at(-1);
        if (latest?.status === "queued" || latest?.status === "running")
          timer = setTimeout(load, 1000);
      } catch (e) {
        if (alive && !scope.signal.aborted) {
          setError(message(e));
          setLoading(false);
        }
      }
    }
    void load();
    return () => {
      alive = false;
      clearTimeout(timer);
    };
  }, [scope, selected, pollEpoch]);
  useEffect(() => {
    const node = messagesRef.current;
    if (node && followMessages.current) node.scrollTop = node.scrollHeight;
  }, [value?.messages.length, run?.status]);
  function choose(id: string) {
    setConsent(false);
    followMessages.current = true;
    activeID.current = id;
    setSelected(id);
    setReply("");
    setAck(false);
  }
  function accept(r: IntentConversation) {
    if (!mounted.current || scope.signal.aborted) return;
    setValue((prev) =>
      prev?.id === r.id && prev.version > r.version ? prev : r,
    );
    setItems((prev) => [r, ...prev.filter((p) => p.id !== r.id)]);
  }
  async function create(goal?: IntentDocument) {
    if (mutation.current) return;
    const originID = activeID.current;
    mutation.current = true;
    setBusy(true);
    setError("");
    try {
      const input = goal
        ? {
            id: crypto.randomUUID(),
            title: goal.metadata.name,
            scope: authoringScope(goal),
            text: goal.spec.description,
            goal,
          }
        : {
            id: crypto.randomUUID(),
            title: draft.name,
            scope: {
              metadata: { name: draft.name, project: draft.project },
              service: draft.service,
              source: draft.source,
              population: intentPopulation(draft.population),
              noDataPolicy: "unknown",
            },
            text: draft.text,
          };
      const r = await api<IntentConversation>(conversationBase, scope, {
        method: "POST",
        body: JSON.stringify(input),
      });
      if (
        !mounted.current ||
        scope.signal.aborted ||
        activeID.current !== originID
      )
        return;
      choose(r.id);
      accept(r);
      setCreateOpen(false);
      setTab("conversations");
    } catch (e) {
      if (
        mounted.current &&
        !scope.signal.aborted &&
        activeID.current === originID
      )
        setError(message(e));
    } finally {
      mutation.current = false;
      if (mounted.current && !scope.signal.aborted) setBusy(false);
    }
  }
  async function mutate(suffix: string, body: unknown) {
    if (!value) return null;
    const id = value.id;
    const r = await api<IntentConversation>(
      conversationBase + "/" + encodeURIComponent(id) + suffix,
      scope,
      { method: "POST", body: JSON.stringify(body) },
    );
    if (!scope.signal.aborted && activeID.current === id) accept(r);
    return r;
  }
  async function action(fn: () => Promise<unknown>) {
    if (mutation.current) return;
    mutation.current = true;
    setBusy(true);
    setError("");
    try {
      await fn();
    } catch (e) {
      if (!scope.signal.aborted) setError(message(e));
    } finally {
      mutation.current = false;
      setBusy(false);
    }
  }
  async function governanceAction(
    suffix: string,
    body: unknown,
    applied = false,
  ) {
    return action(async () => {
      const result = await mutate(suffix, body);
      if (applied && result && !scope.signal.aborted) onApplied();
    });
  }
  async function prepare() {
    if (!value) return;
    const r = await mutate("/runs", {
      id: crypto.randomUUID(),
      expectedVersion: value.version,
      mode,
      modelConsent: consent,
      ...(history
        ? {
            backtest: {
              from: backtestTime(from),
              to: backtestTime(to),
              resolutionSeconds: Number(resolution),
            },
          }
        : {}),
    });
    if (!r) return;
    setPollEpoch((v) => v + 1);
  }
  function exportYAML(name: string, yaml: string) {
    const url = URL.createObjectURL(
      new Blob([yaml], { type: "application/yaml" }),
    );
    const a = document.createElement("a");
    a.href = url;
    a.download = name;
    a.click();
    URL.revokeObjectURL(url);
  }
  const input = (
    key: keyof typeof draft,
    label: string,
    placeholder?: string,
  ) => (
    <div className="grid gap-2" key={key}>
      <Label htmlFor={"flow-" + key}>{label}</Label>
      <Input
        id={"flow-" + key}
        value={draft[key]}
        placeholder={placeholder}
        onChange={(e) => setDraft((d) => ({ ...d, [key]: e.target.value }))}
      />
    </div>
  );
  return (
    <Tabs value={tab} onValueChange={setTab} className="space-y-4">
      <TabsList>
        <TabsTrigger value="conversations">Conversations</TabsTrigger>
        <TabsTrigger value="form">Definition form</TabsTrigger>
      </TabsList>
      <TabsContent value="form">
        <IntentAuthoring
          scope={scope}
          journeys={journeys}
          onStartConversation={(goal) => void create(goal)}
        />
      </TabsContent>
      <TabsContent value="conversations" className="space-y-4">
        <div className="flex flex-wrap items-center gap-3">
          <Label htmlFor="intent-conversation">Saved conversation</Label>
          <Select
            value={selected || "none"}
            onValueChange={(id) => choose(id === "none" ? "" : id)}
            disabled={busy}
          >
            <SelectTrigger id="intent-conversation" className="w-full max-w-sm">
              <SelectValue placeholder="Select conversation" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">Select conversation</SelectItem>
              {items.map((item) => (
                <SelectItem key={item.id} value={item.id}>
                  {item.title}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            variant="outline"
            disabled={!writable || busy}
            onClick={() => setCreateOpen((v) => !v)}
          >
            New conversation
          </Button>
          {value ? (
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => setPollEpoch((v) => v + 1)}
            >
              Refresh conversation
            </Button>
          ) : null}
          {cursor ? (
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => void action(() => refreshList(cursor))}
            >
              Load more
            </Button>
          ) : null}
          {value ? (
            <Badge variant="outline">{value.state.replaceAll("_", " ")}</Badge>
          ) : null}
        </div>
        {error ? (
          <p
            role="alert"
            className="rounded-md border p-3 text-sm text-destructive"
          >
            {error}
          </p>
        ) : null}
        {createOpen ? (
          <Card>
            <CardHeader>
              <CardTitle>New intent conversation</CardTitle>
              <p className="text-sm text-muted-foreground">
                Fix the service and eligible population before asking the agent
                to prepare a measurement. Use the definition form to provide a
                typed goal without a model or to select a critical user journey
                step.
              </p>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid gap-4 md:grid-cols-2">
                {input("name", "SLO name")}
                {input("project", "Project")}
                {input("service", "Service")}
                {input("source", "Configured source ID")}
              </div>
              <div className="grid gap-2">
                <Label htmlFor="flow-population">
                  Eligible population (YAML)
                </Label>
                <Textarea
                  id="flow-population"
                  value={draft.population}
                  onChange={(e) =>
                    setDraft((d) => ({ ...d, population: e.target.value }))
                  }
                  placeholder={'service: "checkout"\nenvironment: "production"'}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="flow-goal">Reliability goal</Label>
                <Textarea
                  id="flow-goal"
                  value={draft.text}
                  maxLength={2000}
                  onChange={(e) =>
                    setDraft((d) => ({ ...d, text: e.target.value }))
                  }
                  placeholder="Describe the user outcome, percentage target and rolling duration."
                />
              </div>
              <p className="text-xs text-muted-foreground">
                Missing data remains unknown. The source and population cannot
                change within a conversation.
              </p>
              <Button
                disabled={busy || !writable}
                onClick={() => void create()}
              >
                Save conversation
              </Button>
            </CardContent>
          </Card>
        ) : null}
        {loading ? (
          <p role="status">Loading conversation…</p>
        ) : value ? (
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Card className="min-w-0 gap-0 py-0 lg:h-[calc(100dvh-19rem)] lg:min-h-80">
              <CardHeader className="border-b p-4">
                <CardTitle className="text-sm">{value.title}</CardTitle>
                <p className="text-xs text-muted-foreground">
                  {value.scope.metadata.project} / {value.scope.service} ·{" "}
                  {value.scope.source}
                </p>
              </CardHeader>
              <CardContent className="flex min-h-0 flex-1 flex-col overflow-hidden p-0">
                <div
                  ref={messagesRef}
                  onScroll={() => {
                    const node = messagesRef.current;
                    if (node)
                      followMessages.current =
                        node.scrollHeight - node.scrollTop - node.clientHeight <
                        80;
                  }}
                  className="min-h-20 flex-1 space-y-4 overflow-auto p-4"
                  aria-label="Intent conversation messages"
                  aria-live="polite"
                >
                  {value.messages.map((m) => (
                    <article key={m.id} className="rounded-md border p-3">
                      <p className="mb-2 text-xs font-medium text-muted-foreground">
                        {m.role === "user"
                          ? m.actor.name || "You"
                          : "Intent workflow"}{" "}
                        · {new Date(m.at).toLocaleTimeString()}
                      </p>
                      <p className="whitespace-pre-wrap break-words text-sm">
                        {m.text}
                      </p>
                    </article>
                  ))}
                  {running ? (
                    <p role="status" className="text-sm text-muted-foreground">
                      Preparing evidence · {run?.steps.length ?? 0} completed
                      tools · {run?.modelCalls ?? 0} model calls
                      {run?.progress ? (
                        <IntentProgress progress={run.progress} />
                      ) : null}
                    </p>
                  ) : null}
                </div>
                <div
                  className="shrink-0 space-y-2 overflow-y-auto border-t p-3 lg:max-h-[65%]"
                  data-replay-block
                >
                  <Label htmlFor="intent-reply">
                    Clarification or revised goal
                  </Label>
                  <Textarea
                    id="intent-reply"
                    className="h-12 min-h-12 resize-none"
                    value={reply}
                    maxLength={2000}
                    rows={2}
                    disabled={!writable || busy || running}
                    onChange={(e) => setReply(e.target.value)}
                  />
                  <div className="flex flex-wrap gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={!writable || busy || running || !reply.trim()}
                      onClick={() =>
                        void action(async () => {
                          await mutate("/messages", {
                            id: crypto.randomUUID(),
                            expectedVersion: value.version,
                            text: reply,
                          });
                          setReply("");
                        })
                      }
                    >
                      Save answer
                    </Button>
                    <Button
                      size="sm"
                      disabled={
                        !writable ||
                        busy ||
                        running ||
                        (sharesModel && !consent)
                      }
                      onClick={() => void action(prepare)}
                    >
                      Prepare proposal
                    </Button>
                    {running ? (
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={!writable || busy}
                        onClick={() =>
                          void action(() =>
                            mutate("/cancel", {
                              expectedVersion: value.version,
                            }),
                          )
                        }
                      >
                        Cancel run
                      </Button>
                    ) : null}
                  </div>
                  <details className="text-xs">
                    <summary className="cursor-pointer font-medium">
                      Run options
                    </summary>
                    <div className="mt-3 grid gap-3">
                      <Label htmlFor="intent-mode">Preparation mode</Label>
                      <Select
                        value={mode}
                        onValueChange={setMode}
                        disabled={busy || running}
                      >
                        <SelectTrigger id="intent-mode">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="guided">
                            Guided preparation
                          </SelectItem>
                          <SelectItem
                            value="model"
                            disabled={!caps?.modelEnabled}
                          >
                            Model-directed agent
                          </SelectItem>
                        </SelectContent>
                      </Select>
                      {!caps?.modelEnabled ? (
                        <p className="text-muted-foreground">
                          Configure a model provider for model-directed runs.
                          Typed goals work with guided preparation.
                        </p>
                      ) : null}
                      <Label className="flex items-start gap-2">
                        <input
                          type="checkbox"
                          checked={history}
                          disabled={busy || running}
                          onChange={(e) => setHistory(e.target.checked)}
                        />
                        <span>Include historical validation</span>
                      </Label>
                      {history ? (
                        <div className="grid gap-2">
                          <Label htmlFor="intent-from">From (RFC 3339)</Label>
                          <Input
                            id="intent-from"
                            value={from}
                            onChange={(e) => setFrom(e.target.value)}
                          />
                          <Label htmlFor="intent-to">To (RFC 3339)</Label>
                          <Input
                            id="intent-to"
                            value={to}
                            onChange={(e) => setTo(e.target.value)}
                          />
                          <Label htmlFor="intent-resolution">
                            Resolution (seconds)
                          </Label>
                          <Input
                            id="intent-resolution"
                            type="number"
                            min="1"
                            max="3600"
                            value={resolution}
                            onChange={(e) => setResolution(e.target.value)}
                          />
                        </div>
                      ) : null}
                    </div>
                  </details>
                  {sharesModel ? (
                    <Label className="flex items-start gap-2 text-xs">
                      <input
                        type="checkbox"
                        checked={consent}
                        disabled={busy || running}
                        onChange={(e) => setConsent(e.target.checked)}
                      />
                      <span>
                        Allow the configured model to receive this goal and
                        measurement metadata. Source credentials, raw samples
                        and generated queries are excluded.
                      </span>
                    </Label>
                  ) : null}
                </div>
              </CardContent>
            </Card>
            <Card className="min-w-0 gap-0 py-0 lg:h-[calc(100dvh-19rem)] lg:min-h-80">
              <CardHeader className="border-b p-4">
                <CardTitle className="text-sm">
                  Measurement and proposal
                </CardTitle>
                <p className="text-xs text-muted-foreground">
                  Preparation writes no SLOs, journeys or recording rules.
                </p>
              </CardHeader>
              <CardContent className="min-h-0 space-y-4 overflow-auto p-4">
                <details className="text-xs">
                  <summary className="cursor-pointer font-medium">
                    Conversation scope
                  </summary>
                  <pre className="mt-2 overflow-auto whitespace-pre-wrap break-all">
                    {stringify(value.scope)}
                  </pre>
                </details>
                {value.goal ? (
                  <dl className="grid grid-cols-2 gap-3 rounded-md border p-3 text-xs">
                    <div>
                      <dt className="text-muted-foreground">Outcome</dt>
                      <dd className="mt-1 capitalize">
                        {value.goal.spec.outcome}
                      </dd>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">Target</dt>
                      <dd className="mt-1">
                        {Number((value.goal.spec.target * 100).toPrecision(12))}
                        %
                      </dd>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">Rolling window</dt>
                      <dd className="mt-1">
                        {duration(value.goal.spec.windowSeconds)}
                      </dd>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">Missing data</dt>
                      <dd className="mt-1">Unknown</dd>
                    </div>
                    {value.goal.spec.thresholdSeconds != null ? (
                      <div>
                        <dt className="text-muted-foreground">
                          Maximum attempt duration
                        </dt>
                        <dd className="mt-1">
                          {value.goal.spec.thresholdSeconds} seconds
                        </dd>
                      </div>
                    ) : null}
                  </dl>
                ) : null}
                {value.goal ? (
                  <details className="text-xs">
                    <summary className="cursor-pointer font-medium">
                      Typed intent
                    </summary>
                    <pre className="mt-2 overflow-auto whitespace-pre-wrap break-all">
                      {stringify(value.goal)}
                    </pre>
                  </details>
                ) : (
                  <p className="text-sm text-muted-foreground">
                    The goal needs typed outcome, target and window fields
                    before discovery.
                  </p>
                )}
                {value.goal && !value.selection ? (
                  <IntentMeasurementEditor
                    key={value.id + ":" + value.version}
                    goal={value.goal}
                    disabled={!canEdit(scope) || busy || running}
                    onSave={(goal) =>
                      action(() =>
                        mutate("/messages", {
                          id: crypto.randomUUID(),
                          expectedVersion: value.version,
                          text: "Updated the SLO measurement. Verify telemetry and review it before confirmation.",
                          goal,
                        }),
                      )
                    }
                  />
                ) : null}
                {value.goal?.spec.measurement &&
                value.plan?.compiled?.measurementDigest &&
                !value.selection ? (
                  <div className="space-y-3 rounded-md border p-3">
                    <Label className="flex items-start gap-2 text-xs">
                      <input
                        type="checkbox"
                        checked={ack}
                        disabled={busy || running}
                        onChange={(e) => setAck(e.target.checked)}
                      />
                      <span>
                        I have reviewed the observation point, success criteria,
                        eligible attempts and retry semantics shown below.
                      </span>
                    </Label>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={!canEdit(scope) || busy || running || !ack}
                      onClick={() =>
                        void action(() =>
                          mutate("/selection", {
                            expectedVersion: value.version,
                            intentDigest: value.plan!.intentDigest,
                            measurementDigest:
                              value.plan!.compiled!.measurementDigest,
                            confirm: true,
                          }),
                        )
                      }
                    >
                      Confirm SLO measurement
                    </Button>
                  </div>
                ) : null}
                {value.plan?.clarifications.map((q, i) => (
                  <p key={i} className="text-sm">
                    {q}
                  </p>
                ))}
                {value.plan?.candidates.length && !value.selection ? (
                  <div className="space-y-3">
                    <Label className="flex items-start gap-2 text-xs">
                      <input
                        type="checkbox"
                        checked={ack}
                        disabled={busy || running}
                        onChange={(e) => setAck(e.target.checked)}
                      />
                      <span>
                        I have reviewed the observation point, success criteria,
                        eligible attempts and retry semantics for the
                        measurement I confirm.
                      </span>
                    </Label>
                    {value.plan.candidates.map((c) => (
                      <div
                        key={c.binding.name}
                        className="space-y-2 rounded-md border p-3 text-sm"
                      >
                        <p className="font-medium">
                          {c.definition.spec.displayName}
                        </p>
                        <p>{c.definition.spec.observationPoint}</p>
                        <p className="break-all text-xs text-muted-foreground">
                          {c.definition.spec.measurement === "latency"
                            ? `Duration at most ${c.definition.spec.thresholdSeconds} seconds`
                            : "Success labels: " +
                              JSON.stringify(c.definition.spec.successLabels)}
                        </p>
                        <p className="text-xs">
                          {c.binding.name} · revision {c.binding.revision}
                        </p>
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={!canEdit(scope) || busy || running || !ack}
                          onClick={() =>
                            void action(() =>
                              mutate("/selection", {
                                expectedVersion: value.version,
                                intentDigest: value.plan!.intentDigest,
                                binding: {
                                  name: c.binding.name,
                                  expectedRevision: c.binding.revision,
                                },
                                confirm: true,
                              }),
                            )
                          }
                        >
                          Confirm measurement
                        </Button>
                      </div>
                    ))}
                  </div>
                ) : null}
                {value.selection ? (
                  <p className="rounded-md border p-3 text-xs">
                    {value.selection.measurementDigest
                      ? "SLO measurement confirmed."
                      : `Shared measurement confirmed: ${value.selection.binding.name} · revision ${value.selection.binding.expectedRevision}.`}{" "}
                    {value.plan?.compiled
                      ? "This measurement was used for the proposal."
                      : "Select Prepare proposal to compile it."}
                  </p>
                ) : null}
                {value.plan?.compiled ? (
                  <div className="space-y-3">
                    <IntentEvidence
                      compiled={value.plan.compiled}
                      sourceDigest={value.plan.sourceDigest}
                    />
                    <p className="text-sm font-medium">SLO proposal</p>
                    <p className="text-xs text-muted-foreground">
                      Expected resource revision {value.plan.expectedRevision}
                    </p>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        exportYAML(
                          value.scope.metadata.name + ".slo.yaml",
                          value.plan!.compiled!.yaml,
                        )
                      }
                    >
                      Download SLO YAML
                    </Button>
                    <pre className="max-h-80 overflow-auto rounded-md border p-3 text-xs">
                      {value.plan.compiled.yaml}
                    </pre>
                    <p className="break-all font-mono text-xs">
                      SHA-256 {value.plan.compiled.digest}
                    </p>
                  </div>
                ) : null}
                {value.plan?.journey ? (
                  <details className="text-xs">
                    <summary className="cursor-pointer font-medium">
                      Critical user journey proposal · revision{" "}
                      {value.plan.journey.expectedRevision}
                    </summary>
                    <pre className="mt-2 max-h-80 overflow-auto rounded-md border p-3">
                      {value.plan.journey.yaml}
                    </pre>
                    <Button
                      className="mt-2"
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        exportYAML(
                          value.plan!.journey!.manifest.metadata.name +
                            ".cuj.yaml",
                          value.plan!.journey!.yaml,
                        )
                      }
                    >
                      Download CUJ YAML
                    </Button>
                  </details>
                ) : null}
                {value.state === "ready" &&
                value.plan?.compiled &&
                !value.changeSet ? (
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={!writable || busy || running}
                    onClick={() =>
                      void action(() =>
                        mutate("/change-set", {
                          expectedVersion: value.version,
                        }),
                      )
                    }
                  >
                    Review changes
                  </Button>
                ) : null}
                {value.changeSet ? (
                  <section
                    className="space-y-3 rounded-md border p-3"
                    aria-label="Reviewed intent changes"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <p className="text-sm font-medium">Change set</p>
                      <Badge variant="outline">{value.changeSet.state}</Badge>
                    </div>
                    <p className="text-xs text-muted-foreground">
                      Applies definitions and CUJ links together. Recording and
                      alert rules require a separate destination review.
                    </p>
                    <IntentRecommendationReport
                      report={value.changeSet.recommendation}
                    />
                    {value.changeSet.changes.map((change) => (
                      <details
                        className="text-xs"
                        key={change.kind + change.name}
                      >
                        <summary className="cursor-pointer font-medium">
                          {change.action}: {change.name} ·{" "}
                          {change.kind === "CriticalUserJourney"
                            ? "Critical user journey"
                            : "SLO"}{" "}
                          · revision {change.expectedRevision}
                        </summary>
                        <div className="mt-2 space-y-2">
                          {change.previous ? (
                            <>
                              <p className="font-medium">Current definition</p>
                              <pre className="max-h-60 overflow-auto rounded-md border p-2">
                                {stringify(change.previous)}
                              </pre>
                            </>
                          ) : (
                            <p>New resource</p>
                          )}
                          <p className="font-medium">Proposed definition</p>
                          <pre className="max-h-60 overflow-auto rounded-md border p-2">
                            {stringify(change.manifest)}
                          </pre>
                        </div>
                      </details>
                    ))}
                    <p className="break-all font-mono text-xs">
                      SHA-256 {value.changeSet.digest}
                    </p>
                    <details className="text-xs">
                      <summary className="cursor-pointer font-medium">
                        Kubernetes and GitOps export
                      </summary>
                      <div className="mt-2 space-y-2">
                        <Label htmlFor="intent-kube-namespace">
                          Kubernetes namespace
                        </Label>
                        <Input
                          id="intent-kube-namespace"
                          value={kubeNamespace}
                          onChange={(e) => setKubeNamespace(e.target.value)}
                          placeholder="team-commerce"
                        />
                        <p className="text-muted-foreground">
                          Download desired definitions for a separate GitOps
                          review. This does not connect to a cluster, deploy
                          rules or transfer definition ownership.
                        </p>
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={
                            busy ||
                            !/^[a-z0-9][a-z0-9-]{0,62}$/.test(kubeNamespace)
                          }
                          onClick={() => void action(exportKubernetes)}
                        >
                          Download Kubernetes YAML
                        </Button>
                      </div>
                    </details>
                    <IntentDecisionControls
                      scope={scope}
                      conversation={value}
                      busy={busy || running}
                      onAction={governanceAction}
                    />
                    {value.changeSet.state === "applied" ? (
                      <>
                        <p className="text-xs">
                          Definitions applied. New observations are required to
                          establish reliability.
                        </p>
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={busy}
                          onClick={() => void action(verifyChanges)}
                        >
                          Verify applied definitions
                        </Button>
                        {verification ? (
                          <p className="text-xs">
                            Verifying{" "}
                            {verification.operation === "rollback"
                              ? "restored"
                              : "applied"}{" "}
                            configuration. Source connection:{" "}
                            {verification.sourceState}
                          </p>
                        ) : null}
                        {verification?.resources.map((r) => (
                          <p className="text-xs" key={r.kind + r.name}>
                            {r.name}: {r.state} · current revision{" "}
                            {r.currentRevision || "missing"} · health {r.health}
                          </p>
                        ))}
                      </>
                    ) : null}
                  </section>
                ) : null}
                <IntentGovernanceHistory
                  scope={scope}
                  conversation={value}
                  busy={busy}
                  running={running}
                  onAction={governanceAction}
                  onRead={(fn) => void action(fn)}
                />
                {value.backtest ? (
                  <div className="rounded-md border p-3 text-xs">
                    <p className="mb-2 font-medium">Historical validation</p>
                    <p className="mb-2">
                      {value.backtest.from} to {value.backtest.to} ·{" "}
                      {value.backtest.resolutionSeconds}s resolution
                    </p>
                    <details className="mb-2">
                      <summary className="cursor-pointer font-medium">
                        Historical evidence provenance
                      </summary>
                      <dl className="mt-2 space-y-2 break-all font-mono">
                        <div>
                          <dt>Intent digest</dt>
                          <dd>
                            {value.backtest.intentDigest ?? "Unavailable"}
                          </dd>
                        </div>
                        <div>
                          <dt>Manifest digest</dt>
                          <dd>
                            {value.backtest.manifestDigest ?? "Unavailable"}
                          </dd>
                        </div>
                        <div>
                          <dt>Source fingerprint</dt>
                          <dd>
                            {value.backtest.sourceDigest ?? "Unavailable"}
                          </dd>
                        </div>
                      </dl>
                    </details>
                    <p>
                      {value.backtest.completeCutoffs} complete ·{" "}
                      {value.backtest.unknownCutoffs} unknown ·{" "}
                      {value.backtest.noTrafficCutoffs} without traffic
                    </p>
                    <p>
                      {value.backtest.targetViolationCutoffs} target crossings ·{" "}
                      {value.backtest.burnThresholdCutoffs} burn crossings
                    </p>
                    <p className="mt-2 text-muted-foreground">
                      Sampled crossings are not incident counts or notification
                      simulations.
                    </p>
                  </div>
                ) : null}
                {value.changeSetHistory?.length ? (
                  <details className="text-xs">
                    <summary className="cursor-pointer font-medium">
                      Previous change sets ({value.changeSetHistory.length})
                    </summary>
                    <p className="mt-2 text-muted-foreground">
                      Previous approvals cannot be used after the goal or
                      preparation changes.
                    </p>
                    {value.changeSetHistory.map((c) => (
                      <p className="mt-2 break-all" key={c.id}>
                        {c.state} · {c.createdAt} · SHA-256 {c.digest}
                      </p>
                    ))}
                  </details>
                ) : null}
                {value.plan?.warnings.map((warning, i) => (
                  <p key={i} className="text-xs text-muted-foreground">
                    {warning}
                  </p>
                ))}
                {value.runs.map((r) => (
                  <details key={r.id} className="text-xs">
                    <summary className="cursor-pointer font-medium">
                      {r.mode === "model" ? "Model-directed run" : "Guided run"}{" "}
                      · {r.status.replaceAll("_", " ")} · {r.steps.length} tools
                    </summary>
                    <p className="mt-2 break-all text-muted-foreground">
                      Skill {r.skill.id}@{r.skill.version} · {r.skill.digest}
                    </p>
                    {r.steps.map((s, i) => (
                      <details key={i} className="mt-2 rounded-md border p-2">
                        <summary className="cursor-pointer">
                          {s.tool.replaceAll("_", " ")} ·{" "}
                          {new Date(s.at).toLocaleTimeString()}
                        </summary>
                        <p className="my-2 break-all font-mono">
                          SHA-256 {s.digest}
                        </p>
                        <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-all">
                          {JSON.stringify(s.result, null, 2)}
                        </pre>
                      </details>
                    ))}
                  </details>
                ))}
              </CardContent>
            </Card>
          </div>
        ) : !createOpen ? (
          <Card>
            <CardContent className="p-8 text-center text-sm text-muted-foreground">
              Start a conversation to clarify a reliability goal, or use the
              definition form to prepare a typed intent.
            </CardContent>
          </Card>
        ) : null}
      </TabsContent>
    </Tabs>
  );
}
