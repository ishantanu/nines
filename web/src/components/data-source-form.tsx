import {
  managedProviders,
  providerCredentialFields,
} from "@/lib/telemetry-providers";
import { FormField } from "@/components/ui/form-layout";
import { useState } from "react";
import { api, message, type RequestScope } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
interface Connection {
  id: string;
  kind: string;
  endpoint: string;
  mimirOrgID?: string;
  version?: number;
  options?: Record<string, string>;
  collectionMode?: string;
  rules?: {
    rulerURL?: string;
    reload?: boolean;
    autoDeploy?: boolean;
    entityGUID?: string;
  };
}
interface Draft {
  nativeEntityGUID: string;
  name: string;
  kind: string;
  url: string;
  mimirOrgID: string;
  credentials: Record<string, string>;
  options: Record<string, string>;
  collectionMode: string;
  clearCredentials: boolean;
  rulesEnabled: boolean;
  autoDeploy: boolean;
  reload: boolean;
  rulerURL: string;
  expectedVersion: number;
}
export function DataSourceForm({
  scope,
  enabled,
  connection,
  onSaved,
  allowedRoutes = {},
}: {
  scope: RequestScope;
  enabled: boolean;
  connection?: Connection;
  onSaved: () => void;
  allowedRoutes?: Record<string, string[]>;
}) {
  const [draft, setDraft] = useState<Draft | null>(null),
    [remove, setRemove] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  function open() {
    setError("");
    setDraft({
      nativeEntityGUID: connection?.rules?.entityGUID ?? "",
      name: connection?.id ?? "",
      kind: connection?.kind ?? "prometheus",
      url: connection?.endpoint ?? "",
      mimirOrgID: connection?.mimirOrgID ?? "",
      credentials: {},
      options: { ...connection?.options },
      collectionMode: connection?.collectionMode || "automatic",
      clearCredentials: false,
      rulesEnabled: !!connection?.rules,
      autoDeploy: connection?.rules?.autoDeploy ?? false,
      reload: connection?.rules?.reload ?? false,
      rulerURL: connection?.rules?.rulerURL ?? "",
      expectedVersion: connection?.version ?? 0,
    });
  }
  async function save() {
    if (!draft) return;
    setBusy(true);
    setError("");
    try {
      await api(
        `/api/connections${connection ? `/${connection.id}` : ""}`,
        scope,
        { method: connection ? "PUT" : "POST", body: JSON.stringify(draft) },
      );
      setDraft(null);
      onSaved();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function removeSource() {
    if (!connection) return;
    setBusy(true);
    setError("");
    try {
      await api(`/api/connections/${connection.id}`, scope, {
        method: "DELETE",
        headers: { "If-Match": `"${connection.version}"` },
      });
      setRemove(false);
      onSaved();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <div className="flex gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={!enabled || scope.role !== "owner"}
          onClick={open}
        >
          {connection
            ? "Edit telemetry connection"
            : "Add telemetry connection"}
        </Button>
        {connection ? (
          <Button
            variant="outline"
            size="sm"
            disabled={!enabled || scope.role !== "owner"}
            onClick={() => {
              setError("");
              setRemove(true);
            }}
          >
            Delete telemetry connection
          </Button>
        ) : null}
      </div>
      <Dialog
        open={!!draft}
        onOpenChange={(open) => {
          if (!open && !busy) setDraft(null);
        }}
      >
        <DialogContent className="max-h-[90svh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              {connection
                ? "Edit telemetry connection"
                : "Add telemetry connection"}
            </DialogTitle>
            <DialogDescription>
              Endpoints and upstream accounts or datasets must be allowed by the
              server for this tenant. Credentials are encrypted and never
              returned. Create a new connection to change a referenced
              measurement source.
            </DialogDescription>
          </DialogHeader>
          {draft ? (
            <form
              className="space-y-5"
              onSubmit={(e) => {
                e.preventDefault();
                void save();
              }}
            >
              <FormField htmlFor="source-name" label="Connection name">
                <Input
                  id="source-name"
                  required
                  maxLength={63}
                  pattern="[a-z0-9][a-z0-9-]*"
                  disabled={!!connection}
                  value={draft.name}
                  onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                />
              </FormField>
              <FormField htmlFor="source-kind" label="Provider">
                <select
                  id="source-kind"
                  className="h-9 w-full rounded-md border bg-background px-3"
                  value={draft.kind}
                  disabled={!!connection}
                  onChange={(e) => {
                    const kind = e.target.value;
                    const provider = managedProviders.find(
                      (p) => p.kind === kind,
                    );
                    const route = allowedRoutes[kind]?.[0] ?? "";
                    setDraft({
                      ...draft,
                      kind,
                      nativeEntityGUID: "",
                      url: provider?.endpoint ?? "",
                      credentials: {},
                      options:
                        kind === "newrelic"
                          ? { accountID: route }
                          : kind === "dash0"
                            ? { dataset: route }
                            : {},
                      collectionMode: ["datadog", "newrelic", "dash0"].includes(
                        kind,
                      )
                        ? "manual"
                        : "automatic",
                      mimirOrgID: "",
                      rulerURL: "",
                      reload: false,
                      rulesEnabled: false,
                      autoDeploy: false,
                      clearCredentials: false,
                    });
                  }}
                >
                  {managedProviders.map((provider) => (
                    <option key={provider.kind} value={provider.kind}>
                      {provider.label}
                    </option>
                  ))}
                </select>
                <p className="mt-2 text-xs text-muted-foreground">
                  Google Cloud Monitoring is configured by a server
                  administrator in the tenant file. Existing connections are
                  available when defining SLOs.
                </p>
              </FormField>
              <FormField htmlFor="source-url" label="Query endpoint">
                <Input
                  id="source-url"
                  type="url"
                  required
                  placeholder={
                    managedProviders.find((p) => p.kind === draft.kind)
                      ?.endpoint
                  }
                  value={draft.url}
                  onChange={(e) => setDraft({ ...draft, url: e.target.value })}
                />
              </FormField>
              {draft.kind === "mimir" ? (
                <>
                  <FormField htmlFor="source-org" label="Mimir organization">
                    <Input
                      id="source-org"
                      required
                      placeholder="playground"
                      value={draft.mimirOrgID}
                      onChange={(e) =>
                        setDraft({ ...draft, mimirOrgID: e.target.value })
                      }
                    />
                  </FormField>
                </>
              ) : null}
              {draft.kind === "newrelic" || draft.kind === "dash0" ? (
                <FormField
                  htmlFor="source-route"
                  label={
                    draft.kind === "newrelic"
                      ? "New Relic account ID"
                      : "Dash0 dataset"
                  }
                  description="Accounts and datasets are authorized in the server tenant configuration."
                >
                  <select
                    id="source-route"
                    required
                    aria-describedby="source-route-description"
                    className="h-9 w-full rounded-md border bg-background px-3"
                    value={
                      draft.options[
                        draft.kind === "newrelic" ? "accountID" : "dataset"
                      ] ?? ""
                    }
                    onChange={(e) =>
                      setDraft({
                        ...draft,
                        options: {
                          [draft.kind === "newrelic" ? "accountID" : "dataset"]:
                            e.target.value,
                        },
                      })
                    }
                  >
                    <option value="">
                      {(allowedRoutes[draft.kind]?.length ?? 0) > 0
                        ? "Select an approved account or dataset"
                        : "No accounts or datasets authorized"}
                    </option>
                    {(allowedRoutes[draft.kind] ?? []).map((route) => (
                      <option key={route} value={route}>
                        {route}
                      </option>
                    ))}
                  </select>
                </FormField>
              ) : null}
              {draft.kind === "victoriametrics" ? (
                <FormField
                  htmlFor="source-vm-route"
                  label="VictoriaMetrics routing"
                  description="Use the single-node origin, or a cluster account/project approved in the server tenant configuration. Do not include /select or /api in the endpoint."
                >
                  <select
                    id="source-vm-route"
                    className="h-9 w-full rounded-md border bg-background px-3"
                    aria-describedby="source-vm-route-description"
                    value={
                      draft.options.accountID
                        ? draft.options.accountID +
                          (draft.options.projectID
                            ? ":" + draft.options.projectID
                            : "")
                        : ""
                    }
                    onChange={(e) => {
                      const [accountID, projectID] = e.target.value.split(":");
                      setDraft({
                        ...draft,
                        options: accountID
                          ? { accountID, ...(projectID ? { projectID } : {}) }
                          : {},
                      });
                    }}
                  >
                    <option value="">Single-node (no cluster route)</option>
                    {(allowedRoutes.victoriametrics ?? []).map((route) => (
                      <option key={route} value={route}>
                        Cluster {route}
                      </option>
                    ))}
                  </select>
                  <p className="mt-2 text-xs text-muted-foreground">
                    Use explicit PromQL or MetricsQL interval-count queries.
                    Metric browsing, guided measurement and recording-rule
                    deployment are unavailable for this adapter.
                  </p>
                </FormField>
              ) : null}
              {providerCredentialFields(draft.kind).map((field) => (
                <FormField
                  key={field.key}
                  htmlFor={`source-credential-${field.key}`}
                  label={`${field.label}${connection ? " (leave blank to retain)" : field.required ? "" : " (optional)"}`}
                >
                  <Input
                    id={`source-credential-${field.key}`}
                    type="password"
                    autoComplete="new-password"
                    maxLength={8192}
                    required={
                      field.required && (!connection || draft.clearCredentials)
                    }
                    value={draft.credentials[field.key] ?? ""}
                    onChange={(e) =>
                      setDraft({
                        ...draft,
                        credentials: {
                          ...draft.credentials,
                          [field.key]: e.target.value,
                        },
                      })
                    }
                  />
                </FormField>
              ))}
              <FormField
                htmlFor="source-collection-mode"
                label="Measurement collection"
                description="Automatic collection queries the provider on the server schedule. Manual collection runs only when explicitly requested."
              >
                <select
                  id="source-collection-mode"
                  className="h-9 w-full rounded-md border bg-background px-3"
                  aria-describedby="source-collection-mode-description"
                  value={draft.collectionMode}
                  onChange={(e) =>
                    setDraft({ ...draft, collectionMode: e.target.value })
                  }
                >
                  <option value="automatic">Automatic</option>
                  <option value="manual">Manual</option>
                </select>
              </FormField>
              {draft.kind === "newrelic" ? (
                <div className="space-y-3">
                  <Label className="form-check">
                    <input
                      type="checkbox"
                      checked={draft.rulesEnabled}
                      disabled={!allowedRoutes["newrelic-entity"]?.length}
                      onChange={(e) =>
                        setDraft({
                          ...draft,
                          rulesEnabled: e.target.checked,
                          nativeEntityGUID: e.target.checked
                            ? draft.nativeEntityGUID ||
                              allowedRoutes["newrelic-entity"]?.[0] ||
                              ""
                            : "",
                          autoDeploy: false,
                          reload: false,
                          rulerURL: "",
                        })
                      }
                    />
                    Enable reviewed native SLO publication
                  </Label>
                  {draft.rulesEnabled ? (
                    <FormField
                      htmlFor="source-native-entity"
                      label="Associated New Relic entity"
                      description="Choose an entity approved by the server administrator. Publication verifies that it belongs to the selected account."
                    >
                      <select
                        id="source-native-entity"
                        className="h-9 w-full rounded-md border bg-background px-3"
                        required
                        aria-describedby="source-native-entity-description"
                        value={draft.nativeEntityGUID}
                        onChange={(e) =>
                          setDraft({
                            ...draft,
                            nativeEntityGUID: e.target.value,
                          })
                        }
                      >
                        <option value="">Select an approved entity</option>
                        {(allowedRoutes["newrelic-entity"] ?? []).map(
                          (guid) => (
                            <option key={guid} value={guid}>
                              {guid}
                            </option>
                          ),
                        )}
                      </select>
                    </FormField>
                  ) : null}
                  <p className="text-xs leading-5 text-muted-foreground">
                    {allowedRoutes["newrelic-entity"]?.length
                      ? "Publish reviewed event-count SLOs on this entity. Requires full-platform access and events-to-metrics management permissions. Saving an SLO does not publish it or create native alert conditions."
                      : "Ask the server administrator to approve a New Relic entity GUID before enabling native publication. Event collection remains available."}
                  </p>
                </div>
              ) : null}
              {draft.kind === "datadog" ? (
                <p className="text-sm text-muted-foreground">
                  Use an application key with metric metadata and timeseries
                  query read permissions. This connection reads COUNT/RATE
                  metrics; Nines evaluates its own SLOs and burn alerts.
                </p>
              ) : null}
              {draft.kind === "newrelic" ? (
                <p className="text-sm text-muted-foreground">
                  Use a user API key with account and NRQL read access, not an
                  ingest/license key. Nines reads event counts and evaluates its
                  own SLOs and burn alerts.
                </p>
              ) : null}
              {draft.kind === "dash0" ? (
                <p className="text-sm text-muted-foreground">
                  Use a read token for the approved dataset. Native Dash0 SLO
                  publishing requires separate server configuration and a
                  distinct publish token.
                </p>
              ) : null}
              {connection ? (
                <Label className="form-check">
                  <input
                    type="checkbox"
                    checked={draft.clearCredentials}
                    onChange={(e) =>
                      setDraft({ ...draft, clearCredentials: e.target.checked })
                    }
                  />
                  Clear saved credentials
                </Label>
              ) : null}
              {draft.kind === "datadog" ? (
                <div className="space-y-2">
                  <Label className="form-check">
                    <input
                      type="checkbox"
                      checked={draft.rulesEnabled}
                      onChange={(e) =>
                        setDraft({
                          ...draft,
                          rulesEnabled: e.target.checked,
                          autoDeploy: false,
                          reload: false,
                          rulerURL: "",
                        })
                      }
                    />
                    Enable reviewed native SLO publication
                  </Label>
                  <p className="text-xs leading-5 text-muted-foreground">
                    Requires slos_read and slos_write on this connection's
                    application key. Publish each reviewed 7 or 30 day SLO from
                    its deployment panel. Saving an SLO does not publish it.
                    Native burn monitors are not created.
                  </p>
                </div>
              ) : null}
              {draft.kind === "prometheus" || draft.kind === "mimir" ? (
                <>
                  <Label className="form-check">
                    <input
                      type="checkbox"
                      checked={draft.rulesEnabled}
                      onChange={(e) =>
                        setDraft({
                          ...draft,
                          rulesEnabled: e.target.checked,
                          autoDeploy: e.target.checked,
                          reload:
                            e.target.checked && draft.kind === "prometheus",
                        })
                      }
                    />
                    Enable recording and alert rule deployment
                  </Label>
                  {draft.rulesEnabled ? (
                    <div className="space-y-2">
                      <Label className="form-check">
                        <input
                          type="checkbox"
                          checked={draft.autoDeploy}
                          onChange={(e) =>
                            setDraft({
                              ...draft,
                              autoDeploy: e.target.checked,
                              reload:
                                draft.kind === "prometheus" &&
                                (e.target.checked || draft.reload),
                            })
                          }
                        />
                        Deploy rules automatically after SLO saves
                      </Label>
                      <p className="text-xs leading-5 text-muted-foreground">
                        Enables reconciliation for existing and future SLOs
                        using this source, including approved intent changes and
                        rollbacks. A model cannot approve an SLO. Owners can
                        pause individual SLOs; runtime failures remain visible.
                      </p>
                    </div>
                  ) : null}
                  {draft.rulesEnabled && draft.kind === "prometheus" ? (
                    <div className="space-y-2">
                      <Label className="form-check">
                        <input
                          type="checkbox"
                          checked={draft.reload}
                          disabled={draft.autoDeploy}
                          onChange={(e) =>
                            setDraft({ ...draft, reload: e.target.checked })
                          }
                        />
                        Reload Prometheus after rule deployment
                      </Label>
                      <p className="text-xs leading-5 text-muted-foreground">
                        Requires a shared rule_files mount and
                        --web.enable-lifecycle on this trusted endpoint. Reload
                        affects the full Prometheus configuration. Runtime
                        evaluation is verified separately.
                      </p>
                    </div>
                  ) : null}
                  {draft.rulesEnabled && draft.kind === "mimir" ? (
                    <>
                      <FormField
                        htmlFor="source-ruler"
                        label="Ruler endpoint (defaults to query endpoint)"
                      >
                        <Input
                          id="source-ruler"
                          type="url"
                          placeholder="http://mimir:9009/prometheus"
                          value={draft.rulerURL}
                          onChange={(e) =>
                            setDraft({ ...draft, rulerURL: e.target.value })
                          }
                        />
                      </FormField>
                    </>
                  ) : null}
                  <p className="text-xs text-muted-foreground">
                    Prometheus uses the server's managed rule directory; Mimir
                    uses its pinned organization. Rule namespaces are assigned
                    by the server. Review SLO definitions before saving. With
                    automatic deployment enabled, saved revisions reconcile
                    without a separate rule apply.
                  </p>
                </>
              ) : null}
              {error ? (
                <p role="alert" className="text-sm text-destructive">
                  {error}
                </p>
              ) : null}
              <div className="dialog-actions">
                <Button
                  type="button"
                  variant="outline"
                  disabled={busy}
                  onClick={() => setDraft(null)}
                >
                  Cancel
                </Button>
                <Button disabled={busy}>
                  {busy ? "Saving..." : "Save telemetry connection"}
                </Button>
              </div>
            </form>
          ) : null}
        </DialogContent>
      </Dialog>
      <Dialog
        open={remove}
        onOpenChange={(open) => {
          if (!open && !busy) setRemove(false);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete telemetry connection</DialogTitle>
            <DialogDescription>
              Delete {connection?.id} from this tenant. Connections referenced
              by an SLO or measurement binding cannot be deleted. This does not
              remove metrics from the provider.
            </DialogDescription>
          </DialogHeader>
          {error ? (
            <p role="alert" className="text-destructive">
              {error}
            </p>
          ) : null}
          <div className="dialog-actions">
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => setRemove(false)}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={busy}
              onClick={removeSource}
            >
              Delete telemetry connection
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
