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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
interface Row {
  userId: string;
  role: string;
  effectiveRole: string;
  version: number;
  user: {
    id: string;
    provider: string;
    subject: string;
    name: string;
    email?: string;
    version?: number;
  };
}
type Draft = {
  id: string;
  provider: string;
  username: string;
  name: string;
  email: string;
  password: string;
  role: string;
  expectedVersion: number;
};
export function Users({
  scope,
  members,
  providers,
  onChange,
  onRole,
}: {
  scope: RequestScope;
  members: Row[];
  providers: { id: string; name: string; type: string }[];
  onChange: () => void;
  onRole: (row: Row) => void;
}) {
  const [draft, setDraft] = useState<Draft | null>(null),
    [remove, setRemove] = useState<Row | null>(null),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const local = providers.filter((p) => p.type === "local");
  const [search, setSearch] = useState("");
  async function save() {
    if (!draft) return;
    setBusy(true);
    setError("");
    try {
      const body = draft.id
        ? {
            name: draft.name,
            email: draft.email,
            password: draft.password,
            expectedVersion: draft.expectedVersion,
          }
        : {
            provider: draft.provider,
            username: draft.username,
            name: draft.name,
            email: draft.email,
            password: draft.password,
            role: draft.role,
            expectedVersion: 0,
          };
      await api(
        `/api/team-access/users${draft.id ? `/${draft.id}` : ""}`,
        scope,
        { method: draft.id ? "PUT" : "POST", body: JSON.stringify(body) },
      );
      setDraft(null);
      onChange();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function removeAccess() {
    if (!remove) return;
    setBusy(true);
    setError("");
    try {
      await api(`/api/team-access/members/${remove.userId}`, scope, {
        method: "DELETE",
        headers: { "If-Match": `"${remove.version}"` },
      });
      setRemove(null);
      onChange();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap justify-between gap-3">
        <Input
          aria-label="Search users"
          placeholder="Search users"
          className="max-w-sm"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <Button
          disabled={!local.length}
          onClick={() => {
            setError("");
            setDraft({
              id: "",
              provider: local[0].id,
              username: "",
              name: "",
              email: "",
              password: "",
              role: "viewer",
              expectedVersion: 0,
            });
          }}
        >
          Add user
        </Button>
      </div>
      {!local.length ? (
        <p className="text-sm text-muted-foreground">
          Local accounts require a configured local authentication provider. Add
          external identities through Members; manage their passwords with the
          identity provider.
        </p>
      ) : null}
      {error && !draft && !remove ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>User</TableHead>
            <TableHead>Authentication</TableHead>
            <TableHead>Role</TableHead>
            <TableHead>Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {members
            .filter((m) =>
              `${m.user.name} ${m.user.email ?? ""} ${m.user.subject}`
                .toLowerCase()
                .includes(search.toLowerCase()),
            )
            .map((m) => (
              <TableRow key={m.userId}>
                <TableCell>
                  <p className="font-medium">{m.user.name}</p>
                  <p className="text-xs text-muted-foreground">
                    {m.user.email || m.user.subject}
                  </p>
                </TableCell>
                <TableCell>
                  {providers.find((p) => p.id === m.user.provider)?.name ??
                    m.user.provider}
                </TableCell>
                <TableCell>{m.effectiveRole}</TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={!local.some((p) => p.id === m.user.provider)}
                      onClick={() => {
                        setError("");
                        setDraft({
                          id: m.userId,
                          provider: m.user.provider,
                          username: m.user.subject,
                          name: m.user.name,
                          email: m.user.email ?? "",
                          password: "",
                          role: m.role,
                          expectedVersion: m.user.version ?? 1,
                        });
                      }}
                    >
                      Edit user
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => onRole(m)}
                    >
                      Edit role
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        setError("");
                        setRemove(m);
                      }}
                    >
                      Remove access
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
        </TableBody>
      </Table>
      <Dialog
        open={!!draft}
        onOpenChange={(open) => {
          if (!open && !busy) setDraft(null);
        }}
      >
        <DialogContent className="max-h-[90svh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              {draft?.id ? "Edit user" : "Add local user"}
            </DialogTitle>
            <DialogDescription>
              Access applies to this tenant. Passwords require 12 to 72 bytes.
              Password changes revoke existing sessions; editing shared
              identities requires ownership of every tenant they can access.
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
              {!draft.id ? (
                <>
                  <Label htmlFor="user-provider">Authentication provider</Label>
                  <select
                    id="user-provider"
                    className="h-9 w-full rounded-md border bg-background px-3"
                    value={draft.provider}
                    onChange={(e) =>
                      setDraft({ ...draft, provider: e.target.value })
                    }
                  >
                    {local.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </select>
                  <Label htmlFor="user-username">Username</Label>
                  <Input
                    id="user-username"
                    required
                    autoComplete="off"
                    maxLength={128}
                    value={draft.username}
                    onChange={(e) =>
                      setDraft({ ...draft, username: e.target.value })
                    }
                  />
                </>
              ) : null}
              <Label htmlFor="user-name">Display name</Label>
              <Input
                id="user-name"
                required
                maxLength={300}
                value={draft.name}
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
              />
              <Label htmlFor="user-email">Email</Label>
              <Input
                id="user-email"
                type="email"
                maxLength={320}
                value={draft.email}
                onChange={(e) => setDraft({ ...draft, email: e.target.value })}
              />
              <Label htmlFor="user-password">
                {draft.id ? "New password (leave blank to retain)" : "Password"}
              </Label>
              <Input
                id="user-password"
                type="password"
                autoComplete="new-password"
                required={!draft.id}
                minLength={12}
                maxLength={72}
                value={draft.password}
                onChange={(e) =>
                  setDraft({ ...draft, password: e.target.value })
                }
              />
              {!draft.id ? (
                <>
                  <Label htmlFor="user-role">Tenant role</Label>
                  <select
                    id="user-role"
                    className="h-9 w-full rounded-md border bg-background px-3"
                    value={draft.role}
                    onChange={(e) =>
                      setDraft({ ...draft, role: e.target.value })
                    }
                  >
                    {["viewer", "agent", "editor", "owner"].map((role) => (
                      <option key={role}>{role}</option>
                    ))}
                  </select>
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
                  {busy ? "Saving..." : "Save user"}
                </Button>
              </div>
            </form>
          ) : null}
        </DialogContent>
      </Dialog>
      <Dialog
        open={!!remove}
        onOpenChange={(open) => {
          if (!open && !busy) setRemove(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Remove tenant access</DialogTitle>
            <DialogDescription>
              Remove {remove?.user.name}'s direct membership and team access
              from {scope.tenant}. Access to other tenants is preserved. SSO
              mapping grants must be changed at their source. The last direct
              owner cannot be removed.
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
              onClick={() => setRemove(null)}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={busy}
              onClick={removeAccess}
            >
              Remove access
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
