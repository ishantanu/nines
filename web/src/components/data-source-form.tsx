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
  rules?: { rulerURL?: string };
}
interface Draft {
  name: string;
  kind: string;
  url: string;
  mimirOrgID: string;
  bearerToken: string;
  clearCredentials: boolean;
  rulesEnabled: boolean;
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
          {connection ? "Edit data source" : "Add data source"}
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
            Delete data source
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
              {connection ? "Edit data source" : "Add data source"}
            </DialogTitle>
            <DialogDescription>
              Connection endpoints and Mimir organizations must be allowed by
              the server for this tenant. Referenced source identities cannot
              change; create a new source and review SLO changes instead.
            </DialogDescription>
          </DialogHeader>
          {draft ? (
            <form
              className="space-y-3"
              onSubmit={(e) => {
                e.preventDefault();
                void save();
              }}
            >
              <Label htmlFor="source-name">Connection name</Label>
              <Input
                id="source-name"
                required
                maxLength={63}
                pattern="[a-z0-9][a-z0-9-]*"
                disabled={!!connection}
                value={draft.name}
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
              />
              <Label htmlFor="source-kind">Provider</Label>
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
                  })
                }
              >
                <option value="prometheus">Prometheus</option>
                <option value="mimir">Mimir</option>
              </select>
              <Label htmlFor="source-url">Query endpoint</Label>
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
              {draft.kind === "mimir" ? (
                <>
                  <Label htmlFor="source-org">Mimir organization</Label>
                  <Input
                    id="source-org"
                    required
                    placeholder="playground"
                    value={draft.mimirOrgID}
                    onChange={(e) =>
                      setDraft({ ...draft, mimirOrgID: e.target.value })
                    }
                  />
                </>
              ) : null}
              <Label htmlFor="source-token">
                Bearer token{" "}
                {connection ? "(leave blank to retain)" : "(optional)"}
              </Label>
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
              {connection ? (
                <Label className="flex items-center gap-2">
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
              <Label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={draft.rulesEnabled}
                  onChange={(e) =>
                    setDraft({ ...draft, rulesEnabled: e.target.checked })
                  }
                />
                Enable recording and alert rule deployment
              </Label>
              {draft.rulesEnabled && draft.kind === "mimir" ? (
                <>
                  <Label htmlFor="source-ruler">
                    Ruler endpoint (defaults to query endpoint)
                  </Label>
                  <Input
                    id="source-ruler"
                    type="url"
                    placeholder="http://mimir:9009/prometheus"
                    value={draft.rulerURL}
                    onChange={(e) =>
                      setDraft({ ...draft, rulerURL: e.target.value })
                    }
                  />
                </>
              ) : null}
              <p className="text-xs text-muted-foreground">
                Prometheus uses the server's managed rule directory; Mimir uses
                its pinned organization. Rule namespaces are assigned by the
                server. Save the source, test it, then review SLO definitions
                and rule deployment separately.
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
                  {busy ? "Saving..." : "Save data source"}
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
            <DialogTitle>Delete data source</DialogTitle>
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
              Delete data source
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
