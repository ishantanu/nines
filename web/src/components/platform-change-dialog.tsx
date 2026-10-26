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
import { Label } from "./ui/label";
import { Textarea } from "./ui/textarea";
import { Button } from "./ui/button";
import type { PlatformUser, PlatformOverview } from "@/lib/platform";
export type PlatformDraft = {
  action: "create" | "profile" | "membership" | "remove" | "revoke" | "policy";
  user?: PlatformUser;
  tenant: string;
  provider: string;
  username: string;
  name: string;
  email: string;
  password: string;
  role: string;
  reason: string;
  enabled: boolean;
  limit: string;
  version: number;
};

function Choice({
  id,
  value,
  disabled,
  onChange,
  options,
}: {
  id: string;
  value: string;
  disabled: boolean;
  onChange: (value: string) => void;
  options: { id: string; name: string }[];
}) {
  return (
    <Select value={value} disabled={disabled} onValueChange={onChange}>
      <SelectTrigger id={id}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.id} value={option.id}>
            {option.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
export function PlatformChangeDialog({
  draft,
  overview,
  busy,
  csrf,
  onChange: set,
  onClose,
  onSave,
}: {
  draft: PlatformDraft;
  overview?: PlatformOverview;
  busy: boolean;
  csrf: string;
  onChange: (value: Partial<PlatformDraft>) => void;
  onClose: () => void;
  onSave: () => Promise<void>;
}) {
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
          <DialogTitle>
            {draft?.action === "policy"
              ? "Jev policy"
              : draft?.action === "create"
                ? "Create local user"
                : draft?.action === "profile"
                  ? "Edit local account"
                  : draft?.action === "membership"
                    ? "Tenant access"
                    : draft?.action === "remove"
                      ? "Remove tenant access"
                      : "Revoke user sessions"}
          </DialogTitle>
          <DialogDescription>
            {draft?.action === "policy"
              ? "Controls apply to future System One requests. Model consent and human approval remain required."
              : "Review the account and tenant scope before saving. A reason is required."}
          </DialogDescription>
        </DialogHeader>
        {draft ? (
          <form
            className="space-y-5"
            onSubmit={(e) => {
              e.preventDefault();
              void onSave();
            }}
          >
            {draft.action !== "revoke" ? (
              <>
                <FormField htmlFor="platform-tenant" label="Tenant">
                  <Choice
                    id="platform-tenant"
                    value={draft.tenant}
                    disabled={busy || draft.action === "policy"}
                    onChange={(value) =>
                      set({
                        tenant: value,
                        ...(draft.action === "membership"
                          ? {
                              role:
                                draft.user?.memberships.find(
                                  (m) => m.tenant === value,
                                )?.member.role ?? "viewer",
                            }
                          : {}),
                      })
                    }
                    options={
                      (draft.action === "remove"
                        ? draft.user?.memberships.map((m) => ({
                            id: m.tenant,
                            name: m.tenant,
                          }))
                        : overview?.tenants) ?? []
                    }
                  />
                </FormField>
              </>
            ) : (
              <p className="text-sm">
                Revoke all current browser and CLI user sessions for{" "}
                {draft.user?.user.name}. This does not revoke independent
                service-account tokens.
              </p>
            )}
            {draft.action === "create" ? (
              <>
                <FormField htmlFor="platform-provider" label="Local provider">
                  <Choice
                    id="platform-provider"
                    value={draft.provider}
                    disabled={busy}
                    onChange={(value) => set({ provider: value })}
                    options={
                      overview?.providers.filter((p) => p.type === "local") ??
                      []
                    }
                  />
                </FormField>
                <FormField htmlFor="platform-username" label="Username">
                  <Input
                    id="platform-username"
                    required
                    value={draft.username}
                    disabled={busy}
                    autoComplete="off"
                    onChange={(e) => set({ username: e.target.value })}
                  />
                </FormField>
              </>
            ) : null}
            {draft.action === "create" || draft.action === "profile" ? (
              <>
                <FormField htmlFor="platform-name" label="Display name">
                  <Input
                    id="platform-name"
                    value={draft.name}
                    required
                    maxLength={300}
                    disabled={busy}
                    onChange={(e) => set({ name: e.target.value })}
                  />
                </FormField>
                <FormField htmlFor="platform-email" label="Email">
                  <Input
                    id="platform-email"
                    value={draft.email}
                    maxLength={320}
                    disabled={busy}
                    onChange={(e) => set({ email: e.target.value })}
                  />
                </FormField>
                <FormField
                  htmlFor="platform-password"
                  label={
                    <>
                      {" "}
                      {draft.action === "profile"
                        ? "New password (leave empty to preserve)"
                        : "Initial password"}{" "}
                    </>
                  }
                >
                  <Input
                    id="platform-password"
                    type="password"
                    autoComplete="new-password"
                    value={draft.password}
                    required={draft.action === "create"}
                    minLength={12}
                    maxLength={72}
                    disabled={busy}
                    onChange={(e) => set({ password: e.target.value })}
                  />
                </FormField>
              </>
            ) : null}
            {draft.action === "create" || draft.action === "membership" ? (
              <>
                <FormField htmlFor="platform-role" label="Tenant role">
                  <Choice
                    id="platform-role"
                    value={draft.role}
                    disabled={busy}
                    onChange={(value) => set({ role: value })}
                    options={["viewer", "agent", "editor", "owner"].map(
                      (role) => ({ id: role, name: role }),
                    )}
                  />
                </FormField>
              </>
            ) : null}
            {draft.action === "membership" || draft.action === "remove" ? (
              <p className="text-sm">
                Identity: {draft.provider} / {draft.username}. Direct grants are
                separate from SSO and team grants. Removing a direct grant does
                not remove upstream SSO entitlement.
              </p>
            ) : null}
            {draft.action === "policy" ? (
              <>
                <p className="text-sm">{draft.name}</p>
                <Label className="form-check">
                  <input
                    type="checkbox"
                    checked={draft.enabled}
                    disabled={
                      busy ||
                      !overview?.tenants.find((t) => t.id === draft.tenant)
                        ?.systemOne
                    }
                    onChange={(e) => set({ enabled: e.target.checked })}
                  />
                  Enable Jev requests
                </Label>
                <FormField
                  htmlFor="platform-daily-limit"
                  label="Daily request limit (UTC)"
                >
                  <Input
                    id="platform-daily-limit"
                    type="number"
                    min={1}
                    max={100000}
                    required
                    value={draft.limit}
                    disabled={busy}
                    onChange={(e) => set({ limit: e.target.value })}
                  />
                </FormField>
                <p className="text-xs text-muted-foreground">
                  Requests reserve quota before the provider call; failed
                  attempts count. Credentials and model routes remain
                  server-owned. Generative model budgets are configured
                  separately.
                </p>
              </>
            ) : null}
            <FormField htmlFor="platform-reason" label="Reason for change">
              <Textarea
                id="platform-reason"
                required
                maxLength={1000}
                value={draft.reason}
                disabled={busy}
                onChange={(e) => set({ reason: e.target.value })}
              />
            </FormField>
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
                disabled={busy || !draft.reason.trim() || !csrf}
              >
                {busy ? "Saving…" : "Save change"}
              </Button>
            </div>
          </form>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
