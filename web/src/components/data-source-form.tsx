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
  rules?: { rulerURL?: string; reload?: boolean; autoDeploy?: boolean };
}
interface Draft {
  name: string;
  kind: string;
  url: string;
  mimirOrgID: string;
  bearerToken: string;
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
}: {
  scope: RequestScope;
  enabled: boolean;
  connection?: Connection;
  onSaved: () => void;
}) {
  const [draft, setDraft] = useState<Draft | null>(null),
    [remove, setRemove] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  function open() {
    setError("");
    setDraft({
      name: connection?.id ?? "",
      kind: connection?.kind ?? "prometheus",
      url: connection?.endpoint ?? "",
      mimirOrgID: connection?.mimirOrgID ?? "",
      bearerToken: "",
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
              Connection endpoints and Mimir organizations must be allowed by
              the server for this tenant. Referenced source identities cannot
              change; create a new source and review SLO changes instead.
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
                  onChange={(e) =>
                    setDraft({
                      ...draft,
                      kind: e.target.value,
                      mimirOrgID: "",
                      rulerURL: "",
                      reload:
                        e.target.value === "prometheus" && draft.autoDeploy,
                    })
                  }
                >
                  <option value="prometheus">Prometheus</option>
                  <option value="mimir">Mimir</option>
                </select>
              </FormField>
              <FormField htmlFor="source-url" label="Query endpoint">
                <Input
                  id="source-url"
                  type="url"
                  required
                  placeholder={
                    draft.kind === "mimir"
                      ? "http://mimir:9009/prometheus"
                      : "http://prometheus:9090"
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
              <FormField
                htmlFor="source-token"
                label={
                  <>
                    {" "}
                    Bearer token{" "}
                    {connection ? "(leave blank to retain)" : "(optional)"}{" "}
                  </>
                }
              >
                <Input
                  id="source-token"
                  type="password"
                  autoComplete="new-password"
                  maxLength={8192}
                  value={draft.bearerToken}
                  onChange={(e) =>
                    setDraft({ ...draft, bearerToken: e.target.value })
                  }
                />
              </FormField>
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
              <Label className="form-check">
                <input
                  type="checkbox"
                  checked={draft.rulesEnabled}
                  onChange={(e) =>
                    setDraft({
                      ...draft,
                      rulesEnabled: e.target.checked,
                      autoDeploy: e.target.checked,
                      reload: e.target.checked && draft.kind === "prometheus",
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
                    Enables reconciliation for existing and future SLOs using
                    this source, including approved intent changes and
                    rollbacks. A model cannot approve an SLO. Owners can pause
                    individual SLOs; runtime failures remain visible.
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
                Prometheus uses the server's managed rule directory; Mimir uses
                its pinned organization. Rule namespaces are assigned by the
                server. Review SLO definitions before saving. With automatic
                deployment enabled, saved revisions reconcile without a separate
                rule apply.
              </p>
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
