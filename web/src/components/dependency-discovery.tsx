import { useEffect, useId, useRef, useState } from "react";
import { api, message, type RequestScope } from "@/lib/api";
import { projectKubernetes } from "@/lib/kubernetes-inventory";
import {
  mergeDependencyDiscovery,
  type DependencyManifest,
} from "@/lib/dependency-editor";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
interface DiscoveryResult {
  tenant: string;
  manifest: DependencyManifest;
  inventoryDigest: string;
  evidence: {
    caller: string;
    callee: string;
    resource: string;
    reason: string;
  }[];
  warnings: string[];
}
export function DependencyDiscovery({
  scope,
  value,
  disabled,
  onChange,
  onAdopt,
}: {
  scope: RequestScope;
  value: DependencyManifest;
  disabled: boolean;
  onChange: (next: DependencyManifest) => void;
  onAdopt: () => void;
}) {
  const id = useId();
  const [capabilities, setCapabilities] = useState<{
    clusterEnabled: boolean;
    namespaces: string[];
  } | null>(null);
  const [mode, setMode] = useState("resources");
  const [namespace, setNamespace] = useState("default");
  const [raw, setRaw] = useState("");
  const [result, setResult] = useState<DiscoveryResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const fileEpoch = useRef(0);
  const pending = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    api<{ clusterEnabled: boolean; namespaces: string[] }>(
      "/api/dependency-discovery/kubernetes",
      scope,
      { signal: controller.signal },
    )
      .then((v) => {
        if (!controller.signal.aborted) setCapabilities(v);
      })
      .catch(() => {});
    return () => {
      fileEpoch.current++;
      controller.abort();
      pending.current?.abort();
    };
  }, [scope]);
  function invalidate() {
    fileEpoch.current++;
    pending.current?.abort();
    setBusy(false);
    setResult(null);
    setError("");
  }
  async function preview() {
    const controller = new AbortController();
    pending.current?.abort();
    pending.current = controller;
    setBusy(true);
    setResult(null);
    setError("");
    try {
      const inventory =
        mode === "resources" ? projectKubernetes(raw, namespace) : undefined;
      const next = await api<DiscoveryResult>(
        "/api/dependency-discovery/kubernetes",
        scope,
        {
          method: "POST",
          signal: controller.signal,
          body: JSON.stringify({
            mode,
            namespace,
            name: value.metadata.name,
            project: value.metadata.project,
            displayName: value.spec.displayName,
            inventory,
          }),
        },
      );
      if (next.tenant !== scope.tenant)
        throw new Error("Discovery returned an unexpected tenant.");
      if (!controller.signal.aborted && !scope.signal.aborted) setResult(next);
    } catch (e) {
      if (!controller.signal.aborted && !scope.signal.aborted)
        setError(message(e));
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  return (
    <div className="space-y-4" data-private data-replay-block>
      <p className="text-sm text-muted-foreground">
        Discover services and configuration-derived links from Kubernetes.
        Previewing does not change the cluster or save a map. Trace-based
        dependencies remain separate observed evidence.
      </p>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor={`${id}-mode`}>Inventory source</Label>
          <select
            id={`${id}-mode`}
            disabled={busy || disabled}
            className="h-9 w-full rounded-md border border-input bg-background px-2 text-sm focus-visible:outline-2 focus-visible:outline-ring"
            value={mode}
            onChange={(e) => {
              invalidate();
              setMode(e.target.value);
              if (e.target.value === "cluster")
                setNamespace(capabilities?.namespaces[0] ?? "");
            }}
          >
            <option value="resources">Local resource YAML or JSON</option>
            <option value="cluster" disabled={!capabilities?.clusterEnabled}>
              Connected Kubernetes cluster
            </option>
          </select>
        </div>
        <div className="space-y-2">
          <Label htmlFor={`${id}-namespace`}>
            {mode === "cluster"
              ? "Allowed namespace"
              : "Default resource namespace"}
          </Label>
          {mode === "cluster" ? (
            <select
              id={`${id}-namespace`}
              value={namespace}
              disabled={busy || disabled}
              onChange={(e) => {
                invalidate();
                setNamespace(e.target.value);
              }}
              className="h-9 w-full rounded-md border border-input bg-background px-2 text-sm focus-visible:outline-2 focus-visible:outline-ring"
            >
              {capabilities?.namespaces.map((ns) => (
                <option key={ns}>{ns}</option>
              ))}
            </select>
          ) : (
            <Input
              id={`${id}-namespace`}
              value={namespace}
              maxLength={63}
              disabled={busy || disabled}
              onChange={(e) => {
                invalidate();
                setNamespace(e.target.value);
              }}
            />
          )}
        </div>
      </div>
      {mode === "resources" ? (
        <>
          <p className="text-sm text-muted-foreground">
            Export Services, workload controllers and Ingress resources. The
            browser strips full URLs, inline credentials and unrelated fields
            before sending inventory to Nines. Secret and ConfigMap contents are
            excluded.
          </p>
          <pre className="code-block overflow-x-auto text-sm">
            kubectl get services,deployments,statefulsets,daemonsets,ingresses
            -n {namespace || "NAMESPACE"} -o yaml
          </pre>
          <Label htmlFor={`${id}-file`}>Open resource file</Label>
          <Input
            id={`${id}-file`}
            type="file"
            accept=".yaml,.yml,.json"
            disabled={busy || disabled}
            onChange={async (e) => {
              invalidate();
              const epoch = fileEpoch.current;
              const file = e.target.files?.[0];
              if (!file) return;
              if (file.size > 2 << 20) {
                setError("Resource export must be at most 2 MiB.");
                return;
              }
              try {
                const content = await file.text();
                if (!scope.signal.aborted && epoch === fileEpoch.current)
                  setRaw(content);
              } catch {
                if (!scope.signal.aborted && epoch === fileEpoch.current)
                  setError("Unable to read the selected file.");
              }
            }}
          />
          <Label htmlFor={`${id}-resources`}>Kubernetes resources</Label>
          <Textarea
            id={`${id}-resources`}
            value={raw}
            disabled={busy || disabled}
            onChange={(e) => {
              invalidate();
              setRaw(e.target.value);
            }}
            spellCheck={false}
            className="min-h-48 font-mono text-sm"
          />
        </>
      ) : (
        <p className="text-sm text-muted-foreground">
          Uses the server's service account and this tenant's explicit namespace
          allowlist. Only Services, Deployments, StatefulSets, DaemonSets and
          Ingress are listed.
        </p>
      )}
      {!capabilities?.clusterEnabled ? (
        <p className="text-sm text-muted-foreground">
          Live cluster discovery is disabled for this tenant. An administrator
          can configure an in-cluster reader with namespace-scoped RBAC.
        </p>
      ) : null}
      <Button
        type="button"
        variant="outline"
        disabled={busy || disabled || (mode === "resources" && !raw.trim())}
        onClick={preview}
      >
        {busy ? "Reading inventory…" : "Preview discovery"}
      </Button>
      {error ? (
        <p role="alert" className="text-sm text-[var(--status-degraded)]">
          {error}
        </p>
      ) : null}
      {result ? (
        <section
          className="space-y-3 rounded-md border p-4"
          aria-label="Kubernetes discovery preview"
        >
          <h3 className="font-semibold">
            {result.manifest.spec.services.length} services ·{" "}
            {result.manifest.spec.dependencies.length} candidate dependencies
          </h3>
          <ul className="list-disc space-y-2 pl-5 text-sm text-muted-foreground">
            {result.warnings.map((w) => (
              <li key={w}>{w}</li>
            ))}
          </ul>
          {!result.evidence.length ? (
            <p className="text-sm">
              No links could be inferred from this inventory. Add dependencies
              in the visual editor.
            </p>
          ) : (
            <div
              role="region"
              aria-label="Kubernetes dependency evidence"
              tabIndex={0}
              className="max-h-64 overflow-auto rounded-sm border focus-visible:outline-2 focus-visible:outline-ring"
            >
              <table className="w-full text-left text-sm">
                <caption className="sr-only">
                  Configuration evidence for proposed dependencies
                </caption>
                <thead>
                  <tr>
                    <th scope="col" className="p-2">
                      Caller
                    </th>
                    <th scope="col" className="p-2">
                      Depends on
                    </th>
                    <th scope="col" className="p-2">
                      Evidence
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {result.evidence.map((e, i) => (
                    <tr key={i} className="border-t">
                      <td className="p-2">{e.caller}</td>
                      <td className="p-2">{e.callee}</td>
                      <td className="p-2">
                        {e.resource}
                        <span className="block text-muted-foreground">
                          {e.reason}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <p className="text-sm text-muted-foreground">
            Add these candidates to the draft, then review links and
            criticality. Existing SLO links, manual dependencies and positions
            are preserved. Saving requires a separate plan and apply.
          </p>
          <details className="text-sm">
            <summary>Discovery receipt</summary>
            <p className="mt-2 break-all font-mono">
              Inventory SHA-256: {result.inventoryDigest}
            </p>
          </details>
          <Button
            type="button"
            disabled={disabled || busy}
            onClick={() => {
              try {
                onChange(mergeDependencyDiscovery(value, result.manifest));
                onAdopt();
              } catch (e) {
                setError(message(e));
              }
            }}
          >
            Add discovery to draft
          </Button>
        </section>
      ) : null}
    </div>
  );
}
