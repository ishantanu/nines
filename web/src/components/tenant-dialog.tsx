import { useState } from "react";
import { FormField } from "@/components/ui/form-layout";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./ui/select";
import { Input } from "./ui/input";
import { Textarea } from "./ui/textarea";
import { Button } from "./ui/button";
import { message } from "@/lib/api";
import {
  platformWrite,
  type PlatformOverview,
  type PlatformTenant,
} from "@/lib/platform";

export type TenantAction = "create" | "rename" | "retire" | "delete";

const titles: Record<TenantAction, string> = {
  create: "Create tenant",
  rename: "Rename tenant",
  retire: "Retire tenant",
  delete: "Delete tenant",
};

// Tenant IDs are lowercase letters, digits and hyphens, as the server checks.
const tenantID = /^[a-z0-9][a-z0-9-]{0,62}$/;

// Create, rename, retire or delete a console tenant. Creating one also gives
// it a first owner, so it is usable at once.
export function TenantDialog({
  action,
  tenant,
  overview,
  csrf,
  onClose,
  onDone,
}: {
  action: TenantAction;
  tenant?: PlatformTenant;
  overview: PlatformOverview;
  csrf: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const local = overview.providers.filter((p) => p.type === "local");
  const [id, setID] = useState(""),
    [name, setName] = useState(tenant?.name ?? ""),
    [owner, setOwner] = useState<"local" | "existing">(
      local.length ? "local" : "existing",
    ),
    [provider, setProvider] = useState(
      local[0]?.id ?? overview.providers[0]?.id ?? "",
    ),
    [username, setUsername] = useState(""),
    [ownerName, setOwnerName] = useState(""),
    [password, setPassword] = useState(""),
    [confirm, setConfirm] = useState(""),
    [reason, setReason] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");

  async function save() {
    setBusy(true);
    setError("");
    try {
      const base = { reason, expectedVersion: tenant?.version ?? 0 };
      if (action === "create") {
        await platformWrite("tenants/create", csrf, {
          reason,
          newTenant: {
            id,
            name,
            ...(owner === "local"
              ? {
                  ownerUser: {
                    provider,
                    username,
                    name: ownerName,
                    password,
                  },
                }
              : { ownerProvider: provider, ownerSubject: username }),
          },
        });
      } else if (action === "rename") {
        await platformWrite("tenants/rename", csrf, {
          ...base,
          tenant: tenant?.id,
          name,
        });
      } else {
        await platformWrite("tenants/" + action, csrf, {
          ...base,
          tenant: tenant?.id,
          ...(action === "delete" ? { confirm } : {}),
        });
      }
      onDone();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }

  const ready =
    reason.trim() !== "" &&
    csrf !== "" &&
    (action === "create"
      ? tenantID.test(id) &&
        id !== "default" &&
        name.trim() !== "" &&
        username !== "" &&
        (owner === "existing" ||
          (ownerName.trim() !== "" && password.length >= 12))
      : action === "rename"
        ? name.trim() !== "" && name !== tenant?.name
        : action === "delete"
          ? confirm === tenant?.id
          : true);

  return (
    <Dialog
      open={true}
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent
        data-private
        data-replay-block
        className="max-h-[90vh] overflow-y-auto"
      >
        <DialogHeader>
          <DialogTitle>{titles[action]}</DialogTitle>
          <DialogDescription>
            {action === "create"
              ? "The tenant is ready as soon as you create it. Its first owner can sign in and invite others."
              : action === "rename"
                ? "People see the new name at once. The tenant ID does not change."
                : action === "retire"
                  ? `${tenant?.name} stops collecting and alerting and becomes read-only. Its data is kept until you delete it.`
                  : `This permanently deletes ${tenant?.name}: its SLOs, measurements, history, members and access keys. It cannot be undone.`}
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-5"
          onSubmit={(e) => {
            e.preventDefault();
            void save();
          }}
        >
          {action === "create" ? (
            <>
              <FormField
                htmlFor="tenant-id"
                label="Tenant ID"
                description="Lowercase letters, digits and hyphens. It cannot be changed later."
              >
                <Input
                  id="tenant-id"
                  required
                  value={id}
                  maxLength={63}
                  disabled={busy}
                  autoComplete="off"
                  onChange={(e) => setID(e.target.value.toLowerCase())}
                />
              </FormField>
            </>
          ) : null}
          {action === "create" || action === "rename" ? (
            <FormField htmlFor="tenant-name" label="Display name">
              <Input
                id="tenant-name"
                required
                value={name}
                maxLength={200}
                disabled={busy}
                onChange={(e) => setName(e.target.value)}
              />
            </FormField>
          ) : null}
          {action === "create" ? (
            <>
              <FormField htmlFor="tenant-owner" label="First owner">
                <Select
                  value={owner}
                  disabled={busy}
                  onValueChange={(value) => {
                    const next = value as "local" | "existing";
                    setOwner(next);
                    setProvider(
                      (next === "local" ? local[0] : overview.providers[0])
                        ?.id ?? "",
                    );
                  }}
                >
                  <SelectTrigger id="tenant-owner">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {local.length ? (
                      <SelectItem value="local">New local account</SelectItem>
                    ) : null}
                    <SelectItem value="existing">
                      Someone who can already sign in
                    </SelectItem>
                  </SelectContent>
                </Select>
              </FormField>
              <FormField htmlFor="tenant-provider" label="Sign-in provider">
                <Select
                  value={provider}
                  disabled={busy}
                  onValueChange={setProvider}
                >
                  <SelectTrigger id="tenant-provider">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(owner === "local" ? local : overview.providers).map(
                      (p) => (
                        <SelectItem key={p.id} value={p.id}>
                          {p.name}
                        </SelectItem>
                      ),
                    )}
                  </SelectContent>
                </Select>
              </FormField>
              <FormField
                htmlFor="tenant-username"
                label={
                  owner === "local" ? "Username" : "User ID at the provider"
                }
              >
                <Input
                  id="tenant-username"
                  required
                  value={username}
                  maxLength={500}
                  disabled={busy}
                  autoComplete="off"
                  onChange={(e) => setUsername(e.target.value)}
                />
              </FormField>
              {owner === "local" ? (
                <>
                  <FormField htmlFor="tenant-owner-name" label="Owner's name">
                    <Input
                      id="tenant-owner-name"
                      required
                      value={ownerName}
                      maxLength={300}
                      disabled={busy}
                      onChange={(e) => setOwnerName(e.target.value)}
                    />
                  </FormField>
                  <FormField
                    htmlFor="tenant-password"
                    label="Initial password"
                    description="12 to 72 characters. Share it with the owner securely."
                  >
                    <Input
                      id="tenant-password"
                      type="password"
                      autoComplete="new-password"
                      required
                      minLength={12}
                      maxLength={72}
                      value={password}
                      disabled={busy}
                      onChange={(e) => setPassword(e.target.value)}
                    />
                  </FormField>
                </>
              ) : null}
            </>
          ) : null}
          {action === "delete" ? (
            <FormField
              htmlFor="tenant-confirm"
              label={`Type ${tenant?.id} to confirm`}
            >
              <Input
                id="tenant-confirm"
                required
                value={confirm}
                disabled={busy}
                autoComplete="off"
                onChange={(e) => setConfirm(e.target.value)}
              />
            </FormField>
          ) : null}
          <FormField htmlFor="tenant-reason" label="Reason for change">
            <Textarea
              id="tenant-reason"
              required
              maxLength={1000}
              value={reason}
              disabled={busy}
              onChange={(e) => setReason(e.target.value)}
            />
          </FormField>
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
              onClick={() => onClose()}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              variant={action === "delete" ? "destructive" : undefined}
              disabled={busy || !ready}
            >
              {busy ? "Saving…" : titles[action]}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
