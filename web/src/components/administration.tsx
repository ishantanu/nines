import { DataSourceForm } from "./data-source-form";
import { lazy, Suspense, useEffect, useState } from "react";
import { Check, Minus, RefreshCw, PlugZap } from "lucide-react";
import { api, message, type RequestScope } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
const TeamAccess = lazy(() =>
  import("./team-access").then((m) => ({ default: m.TeamAccess })),
);
interface Access {
  userManagementSupported: boolean;
  currentRole: string;
  authentication: string;
  management: string;
  bindingsVisible: boolean;
  bindings: { credentialRef: string; role: string }[];
  permissions: { id: string; label: string; roles: Record<string, boolean> }[];
}
interface Connections {
  managedConnectionsEnabled?: boolean;
  management: string;
  connections: {
    id: string;
    configuration?: string;
    version?: number;
    kind: string;
    endpoint: string;
    mimirOrgID: string;
    authentication: string;
    rules?: {
      namespace: string;
      autoDeploy?: boolean;
      rulerURL?: string;
      directory?: string;
      mode?: string;
      publisher?: string;
    };
  }[];
}
function useSettings<T>(path: string, scope: RequestScope, refresh = 0) {
  const [value, setValue] = useState<T | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    const c = new AbortController();
    api<T>(path, scope, { signal: c.signal })
      .then(setValue)
      .catch((e) => {
        if (!c.signal.aborted && !scope.signal.aborted) setError(message(e));
      });
    return () => c.abort();
  }, [path, scope, refresh]);
  return { value, error };
}
export function AccessControl({ scope }: { scope: RequestScope }) {
  const { value, error } = useSettings<Access>("/api/access", scope);
  if (error)
    return (
      <p role="alert" className="text-sm text-destructive">
        {error}
      </p>
    );
  if (!value)
    return (
      <p role="status" className="empty-state">
        Loading access policy…
      </p>
    );
  return (
    <div className="space-y-4">
      {value.userManagementSupported && scope.role === "owner" ? (
        <Suspense
          fallback={
            <p role="status" className="empty-state">
              Loading team administration…
            </p>
          }
        >
          <TeamAccess scope={scope} />
        </Suspense>
      ) : null}
      <Card className="gap-0 py-0">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b px-5 py-4">
          <h2 className="text-sm font-semibold">Tenant permissions</h2>
          <Badge variant="outline">Your role: {value.currentRole}</Badge>
        </div>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Permission</TableHead>
                {["owner", "editor", "agent", "viewer"].map((r) => (
                  <TableHead className="capitalize" key={r}>
                    {r}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {value.permissions.map((p) => (
                <TableRow key={p.id}>
                  <TableCell>{p.label}</TableCell>
                  {["owner", "editor", "agent", "viewer"].map((r) => (
                    <TableCell key={r}>
                      {p.roles[r] ? (
                        <span className="inline-flex items-center gap-1">
                          <Check className="size-3.5 text-emerald-700" />
                          Allowed
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1 text-muted-foreground">
                          <Minus className="size-3.5" />
                          Denied
                        </span>
                      )}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
      <Card className="gap-0 py-0">
        <div className="panel-header">
          <h2 className="text-sm font-semibold">Credential grants</h2>
        </div>
        <CardContent className="space-y-4 p-5">
          {value.authentication === "anonymous_local" ? (
            <p className="text-sm text-amber-900">
              This local instance permits anonymous owner access. No
              authenticated users or service accounts are configured.
            </p>
          ) : null}
          {value.bindingsVisible ? (
            value.bindings.length ? (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Server credential reference</TableHead>
                    <TableHead>Role</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {value.bindings.map((b, i) => (
                    <TableRow key={i}>
                      <TableCell className="font-mono text-xs">
                        {b.credentialRef}
                      </TableCell>
                      <TableCell className="capitalize">{b.role}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            ) : (
              <p className="text-sm text-muted-foreground">
                No token grants configured.
              </p>
            )
          ) : (
            <p className="text-sm text-muted-foreground">
              Owner access is required to inspect configured grants.
            </p>
          )}
          <p className="text-sm text-muted-foreground">
            {value.userManagementSupported
              ? "Individual identities, memberships, teams and service accounts are managed above. Legacy API-token grants remain in the server tenant configuration until an administrator removes them."
              : "Legacy API-token grants are managed in the server tenant configuration. Configure local accounts, OIDC or LDAP authentication to enable individual login and team administration."}
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
export function Connections({ scope }: { scope: RequestScope }) {
  const [refresh, setRefresh] = useState(0);
  const { value, error } = useSettings<Connections>(
    "/api/connections",
    scope,
    refresh,
  );
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState("");
  async function test(id: string) {
    setBusy(true);
    setResult("");
    try {
      const r = await api<{ status: string }>(
        `/api/connections/${id}/test`,
        scope,
        { method: "POST" },
      );
      setResult(
        r.status === "reachable"
          ? `${id}: configured endpoint responded to a test query. No samples were stored.`
          : "Connection test failed.",
      );
    } catch (e) {
      setResult(message(e));
    } finally {
      setBusy(false);
    }
  }
  if (error)
    return (
      <p role="alert" className="text-sm text-destructive">
        {error}
      </p>
    );
  if (!value)
    return (
      <p role="status" className="empty-state">
        Loading connections…
      </p>
    );
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          Tenant metrics connections and recording-rule destinations.
        </p>
        <DataSourceForm
          scope={scope}
          enabled={!!value.managedConnectionsEnabled}
          onSaved={() => setRefresh((n) => n + 1)}
        />
      </div>
      {!value.managedConnectionsEnabled ? (
        <p className="text-xs text-muted-foreground">
          Source management is disabled. A server administrator must configure
          NINES_SOURCE_HOSTS and an encryption key for credentials.
        </p>
      ) : null}
      {value.connections.length ? (
        <div className="panel-grid">
          {value.connections.map((c) => (
            <Card className="gap-0 py-0" key={c.id}>
              <div className="panel-header">
                <h2 className="text-sm font-semibold">
                  Metrics source: {c.id}
                </h2>
                <Badge variant="secondary" className="capitalize">
                  {c.kind}
                </Badge>
              </div>
              <CardContent className="space-y-4 p-5">
                <dl className="grid gap-4 text-sm sm:grid-cols-2">
                  {[
                    {
                      label: "Endpoint",
                      value: c.endpoint || "Server configured",
                    },
                    {
                      label: "Authentication",
                      value: c.authentication || "Server configured",
                    },
                    ...(c.rules
                      ? [
                          { label: "Rule namespace", value: c.rules.namespace },
                          {
                            label: "Rule deployment",
                            value: c.rules.autoDeploy
                              ? "Automatic after SLO saves"
                              : "Manual reviewed apply",
                          },
                          {
                            label: "Rule destination",
                            value:
                              c.rules.mode === "operator"
                                ? `Kubernetes operator (${c.rules.publisher})`
                                : (c.rules.rulerURL ??
                                  c.rules.directory ??
                                  "Not configured"),
                          },
                        ]
                      : []),
                    ...(c.mimirOrgID
                      ? [{ label: "Mimir tenant", value: c.mimirOrgID }]
                      : []),
                  ].map((x) => (
                    <div key={x.label}>
                      <dt className="text-xs text-muted-foreground">
                        {x.label}
                      </dt>
                      <dd className="mt-1 break-all font-medium">{x.value}</dd>
                    </div>
                  ))}
                </dl>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={busy || scope.role !== "owner"}
                  onClick={() => test(c.id)}
                >
                  <RefreshCw
                    className={busy ? "size-4 animate-spin" : "size-4"}
                  />
                  {busy ? "Testing…" : "Test connection"}
                </Button>
                {c.configuration === "managed" ? (
                  <DataSourceForm
                    scope={scope}
                    enabled={!!value.managedConnectionsEnabled}
                    connection={c}
                    onSaved={() => setRefresh((n) => n + 1)}
                  />
                ) : (
                  <p className="text-xs text-muted-foreground">
                    Managed by server configuration
                  </p>
                )}
                {result ? (
                  <p role="status" className="text-sm">
                    {result}
                  </p>
                ) : null}
              </CardContent>
            </Card>
          ))}
        </div>
      ) : (
        <Card>
          <CardContent className="empty-state">
            <PlugZap className="mx-auto mb-3 size-6" />
            <h2 className="font-semibold text-foreground">
              No data source connected
            </h2>
            <p className="mt-2">
              Connect Prometheus or Mimir to collect SLO measurements. Manual
              interval ingestion is available.
            </p>
          </CardContent>
        </Card>
      )}
      <Card className="gap-0 py-0">
        <div className="panel-header">
          <h2 className="text-sm font-semibold">Connection configuration</h2>
        </div>
        <CardContent className="space-y-3 p-5">
          <p className="text-sm text-muted-foreground">
            Add managed connections here when enabled by a server administrator.
            Named connections route each SLO to its selected dataset and rule
            destination. Managed bearer tokens are encrypted and are never
            returned to the browser. Server-configured connections remain
            read-only.
          </p>
          <pre className="code-block">{`# Inside a tenant in your server configuration:\nconnections:\n  - name: production\n    kind: mimir\n    url: https://mimir.example.com/prometheus\n    mimirOrgID: production\n    bearerTokenEnv: MIMIR_READ_TOKEN\n    rules:\n      namespace: nines\n      rulerURL: https://mimir-ruler.example.com/prometheus\n      bearerTokenEnv: MIMIR_RULES_TOKEN\n\n# In the SLO YAML: spec.source: production\n# Prometheus: kind: prometheus; rules.directory: /etc/prometheus/rules\n# Apply server configuration and restart to change routing.`}</pre>
          <p className="text-xs text-muted-foreground">
            This example is for server-managed connections. Use Add data source
            above to save a managed connection.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
export function Clients({ scope }: { scope: RequestScope }) {
  return (
    <div className="space-y-4">
      <Card className="gap-0 py-0">
        <div className="panel-header">
          <h2 className="text-sm font-semibold">Go SDK</h2>
        </div>
        <CardContent className="space-y-3 p-5">
          <p className="text-sm text-muted-foreground">
            Use the public client package from another Go application. The
            client binds every request to a tenant and sends definitions as
            YAML.
          </p>
          <pre className="code-block">{`import (\n  v1 "github.com/ishantanu/nines/api/v1alpha1"\n  "github.com/ishantanu/nines/client"\n)\n\nc, err := client.New(client.Config{\n  URL: os.Getenv("NINES_URL"),\n  Tenant: os.Getenv("NINES_TENANT"),\n  Token: os.Getenv("API_TOKEN"),\n})\n// Handle err before use.\nslo := v1.NewSLO("checkout", "production", v1.SLOSpec{\n  DisplayName: "Checkout success", Service: "payments",\n  Target: 0.999, WindowSeconds: 86400, StepSeconds: 60,\n})\nplan, err := c.Plan(ctx, slo)\n// Review the plan and handle err before applying.\nrevision, err := c.Apply(ctx, slo, plan.ExpectedRevision)`}</pre>
          <p className="text-xs text-muted-foreground">
            Build from this repository or import
            github.com/ishantanu/nines/client from a published revision. See
            examples/go-client for a runnable example.
          </p>
        </CardContent>
      </Card>
      <Card className="gap-0 py-0">
        <div className="panel-header">
          <h2 className="text-sm font-semibold">CLI and HTTP API</h2>
        </div>
        <CardContent className="space-y-3 p-5">
          <pre className="code-block">{`# Build once: make cli\nexport NINES_TENANT=${scope.tenant}\nexport API_TOKEN='<token supplied by your administrator>'\nbin/nines plan -f objective.yaml -server "$NINES_URL"\nbin/nines apply -f objective.yaml -server "$NINES_URL" \\\n  -expected-revision 0\n\n# Equivalent HTTP create (If-Match 0 creates a missing name):\ncurl "$NINES_URL/api/resources/apply" \\\n  -H "Authorization: Bearer $API_TOKEN" \\\n  -H "X-Nines-Tenant: $NINES_TENANT" \\\n  -H 'Content-Type: application/yaml' -H 'If-Match: "0"' \\\n  --data-binary @objective.yaml`}</pre>
          <p className="text-sm text-muted-foreground">
            Both SLOs and CUJs use nines.dev/v1alpha1. Owners may apply; agents
            and viewers may plan. Targets, rolling windows and burn policies
            support revision-checked updates. Queries, source and sampling
            interval require a new objective.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
