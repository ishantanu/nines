import { Users } from "./users";
import { useEffect, useState } from "react";
import { api, message, type RequestScope } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
type Role = "owner" | "editor" | "agent" | "viewer";
interface Member {
  userId: string;
  role: Role;
  effectiveRole: Role;
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
interface Team {
  id: string;
  name: string;
  role: Role;
  members: string[];
  version: number;
}
interface Account {
  id: string;
  name: string;
  role: Role;
  expiresAt: string;
  createdAt: string;
}
interface SSOMapping {
  id: string;
  provider: string;
  claim: "groups" | "roles";
  value: string;
  role: Role;
  claimsMaxAgeMinutes: number;
}
interface SSOMember {
  user: Member["user"];
  mappedRole: Role;
  effectiveRole: Role;
  mappingIds: string[];
  expiresAt: string;
}
interface Directory {
  ssoMappings?: SSOMapping[];
  ssoMembers?: SSOMember[];
  ssoMembersTruncated?: boolean;
  members: Member[];
  teams: Team[];
  serviceAccounts: Account[];
  providers: { id: string; name: string; type: string }[];
}
interface Audit {
  id: string;
  at: string;
  actorId: string;
  actorKind: string;
  actorName: string;
  action: string;
  resource: string;
  outcome: string;
  requestId?: string;
  revision?: string;
}
interface AuditPage {
  items: Audit[];
  nextCursor: string;
}
interface Draft {
  kind: "member" | "team" | "account";
  id: string;
  name: string;
  provider: string;
  subject: string;
  role: Role;
  members: string[];
  version: number;
  expires: string;
}
const field = "mt-2 h-9 w-full rounded-md border bg-white px-3 text-sm";
function localDate(date: Date) {
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16);
}
export function TeamAccess({ scope }: { scope: RequestScope }) {
  const [directory, setDirectory] = useState<Directory | null>(null);
  const [audit, setAudit] = useState<AuditPage>({ items: [], nextCursor: "" });
  const [refresh, setRefresh] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [draft, setDraft] = useState<Draft | null>(null);
  const [secret, setSecret] = useState("");
  const [remove, setRemove] = useState<{
    path: string;
    name: string;
    version?: number;
  } | null>(null);
  const [search, setSearch] = useState("");
  useEffect(() => {
    const c = new AbortController();
    setError("");
    Promise.all([
      api<Directory>("/api/team-access", scope, { signal: c.signal }),
      api<AuditPage>("/api/team-access/audit", scope, { signal: c.signal }),
    ])
      .then(([d, a]) => {
        if (!c.signal.aborted) {
          setDirectory(d);
          setAudit(a);
        }
      })
      .catch((e) => {
        if (!c.signal.aborted) setError(message(e));
      });
    return () => c.abort();
  }, [scope, refresh]);
  function begin(kind: Draft["kind"], entry?: Member | Team) {
    setError("");
    setDraft({
      kind,
      id: entry && "id" in entry ? entry.id : "",
      name: entry && "name" in entry ? entry.name : "",
      provider:
        entry && "user" in entry
          ? entry.user.provider
          : (directory?.providers[0]?.id ?? ""),
      subject: entry && "user" in entry ? entry.user.subject : "",
      role: entry?.role ?? (kind === "account" ? "agent" : "viewer"),
      members: entry && "members" in entry ? [...entry.members] : [],
      version: entry?.version ?? 0,
      expires: localDate(new Date(Date.now() + 90 * 86400000)),
    });
  }
  async function save() {
    if (!draft) return;
    setBusy(true);
    setError("");
    try {
      if (draft.kind === "member")
        await api("/api/team-access/members", scope, {
          method: "POST",
          body: JSON.stringify({
            provider: draft.provider,
            subject: draft.subject,
            role: draft.role,
            expectedVersion: draft.version,
          }),
        });
      else if (draft.kind === "team")
        await api("/api/team-access/teams", scope, {
          method: "POST",
          body: JSON.stringify({
            id: draft.id,
            name: draft.name,
            role: draft.role,
            members: draft.members,
            expectedVersion: draft.version,
          }),
        });
      else {
        const result = await api<{ token: string }>(
          "/api/team-access/service-accounts",
          scope,
          {
            method: "POST",
            body: JSON.stringify({
              id: draft.id,
              name: draft.name,
              role: draft.role,
              expiresAt: new Date(draft.expires).toISOString(),
            }),
          },
        );
        setSecret(result.token);
      }
      setDraft(null);
      setRefresh((n) => n + 1);
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function confirmRemove() {
    if (!remove) return;
    setBusy(true);
    setError("");
    try {
      await api(remove.path, scope, {
        method: "DELETE",
        headers: remove.version
          ? { "If-Match": `"${remove.version}"` }
          : undefined,
      });
      setRemove(null);
      setRefresh((n) => n + 1);
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function older() {
    if (!audit.nextCursor) return;
    setBusy(true);
    try {
      const page = await api<AuditPage>(
        `/api/team-access/audit?before=${encodeURIComponent(audit.nextCursor)}`,
        scope,
      );
      setAudit((a) => ({
        items: [...a.items, ...page.items],
        nextCursor: page.nextCursor,
      }));
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  if (!directory)
    return (
      <p
        role={error ? "alert" : "status"}
        className={error ? "text-sm text-destructive" : "empty-state"}
      >
        {error || "Loading team access…"}
      </p>
    );
  const rows = audit.items.filter((e) =>
    JSON.stringify([
      e.actorName,
      e.actorKind,
      e.actorId,
      e.action,
      e.resource,
      e.outcome,
    ])
      .toLowerCase()
      .includes(search.toLowerCase()),
  );
  return (
    <Card className="gap-0 py-0">
      <div className="panel-header">
        <div>
          <h2 className="section-label">People and teams</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            Membership and roles apply to this workspace: {scope.tenant}.
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          disabled={busy}
          onClick={() => setRefresh((n) => n + 1)}
        >
          Refresh access
        </Button>
      </div>
      <CardContent className="space-y-4 p-5">
        {error && !draft && !remove ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        <Tabs defaultValue="users">
          <TabsList>
            <TabsTrigger value="users">
              Users ({directory.members.length})
            </TabsTrigger>
            <TabsTrigger value="members">
              Members ({directory.members.length})
            </TabsTrigger>
            <TabsTrigger value="teams">
              Teams ({directory.teams.length})
            </TabsTrigger>
            <TabsTrigger value="sso">SSO mappings</TabsTrigger>
            <TabsTrigger value="accounts">Service accounts</TabsTrigger>
            <TabsTrigger value="audit">Audit log</TabsTrigger>
          </TabsList>
          <TabsContent value="users">
            <Users
              scope={scope}
              members={directory.members}
              providers={directory.providers}
              onChange={() => setRefresh((n) => n + 1)}
              onRole={(user) => begin("member", user as Member)}
            />
          </TabsContent>
          <TabsContent value="members" className="space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <p className="max-w-xl text-xs leading-5 text-muted-foreground">
                Owners manage access, approve changes and deploy rules. Editors
                manage definitions and samples. Agents propose changes and
                report evidence. Viewers inspect and plan. Project-specific
                permissions are not configured in this release.
              </p>
              <Button size="sm" onClick={() => begin("member")}>
                Add member
              </Button>
            </div>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Person / identity</TableHead>
                  <TableHead>Direct role</TableHead>
                  <TableHead>Effective role</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {directory.members.map((m) => (
                  <TableRow key={m.userId}>
                    <TableCell>
                      <p className="font-medium">{m.user.name}</p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {m.user.provider} · {m.user.email || m.user.subject}
                      </p>
                      <p
                        className="mt-1 max-w-72 truncate font-mono text-xs text-muted-foreground"
                        title={m.user.subject}
                      >
                        {m.user.subject}
                      </p>
                    </TableCell>
                    <TableCell className="capitalize">{m.role}</TableCell>
                    <TableCell>
                      <Badge variant="outline" className="capitalize">
                        {m.effectiveRole}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <div className="flex justify-end gap-2">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => begin("member", m)}
                        >
                          Edit role
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() =>
                            setRemove({
                              path: `/api/team-access/members/${m.userId}`,
                              name: m.user.name,
                              version: m.version,
                            })
                          }
                        >
                          Remove
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {!directory.members.length ? (
              <p className="empty-state">
                No individual memberships. Add an exact identity subject or
                configure initial owner bootstrap.
              </p>
            ) : null}
          </TabsContent>
          <TabsContent value="teams" className="space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <p className="max-w-xl text-xs leading-5 text-muted-foreground">
                Teams grant an additional role to existing workspace members.
                They cannot grant owner access. Removing a person from the
                workspace also removes their team memberships.
              </p>
              <Button size="sm" onClick={() => begin("team")}>
                Create team
              </Button>
            </div>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Team</TableHead>
                  <TableHead>Granted role</TableHead>
                  <TableHead>Members</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {directory.teams.map((t) => (
                  <TableRow key={t.id}>
                    <TableCell>
                      <p className="font-medium">{t.name}</p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {t.id}
                      </p>
                    </TableCell>
                    <TableCell className="capitalize">{t.role}</TableCell>
                    <TableCell>{t.members.length}</TableCell>
                    <TableCell>
                      <div className="flex justify-end gap-2">
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => begin("team", t)}
                        >
                          Edit team
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() =>
                            setRemove({
                              path: `/api/team-access/teams/${t.id}`,
                              name: t.name,
                              version: t.version,
                            })
                          }
                        >
                          Remove
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {!directory.teams.length ? (
              <p className="empty-state">No teams in this workspace.</p>
            ) : null}
          </TabsContent>
          <TabsContent value="accounts" className="space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <p className="max-w-xl text-xs leading-5 text-muted-foreground">
                Use separate tenant-scoped credentials for CLI, MCP and
                automation. Tokens have an expiry, are shown once, and can be
                revoked. Service accounts cannot be owners or approve changes.
              </p>
              <Button size="sm" onClick={() => begin("account")}>
                Create service account
              </Button>
            </div>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Account</TableHead>
                  <TableHead>Role</TableHead>
                  <TableHead>Expires</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {directory.serviceAccounts.map((a) => (
                  <TableRow key={a.id}>
                    <TableCell>
                      <p className="font-medium">{a.name}</p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {a.id}
                      </p>
                    </TableCell>
                    <TableCell className="capitalize">{a.role}</TableCell>
                    <TableCell>
                      {new Date(a.expiresAt).toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() =>
                          setRemove({
                            path: `/api/team-access/service-accounts/${a.id}`,
                            name: a.name,
                          })
                        }
                      >
                        Revoke
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {!directory.serviceAccounts.length ? (
              <p className="empty-state">No managed service accounts.</p>
            ) : null}
          </TabsContent>
          <TabsContent value="sso" className="space-y-4">
            <p className="max-w-3xl text-xs leading-5 text-muted-foreground">
              Mappings are configured on the server. Exact claims from verified
              sign-in tokens grant access to this workspace. The highest direct,
              team or SSO role applies. Removing a direct membership does not
              remove an independent SSO grant.
            </p>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Provider / mapping</TableHead>
                  <TableHead>Claim / exact value</TableHead>
                  <TableHead>Role</TableHead>
                  <TableHead>Reauthentication limit</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(directory.ssoMappings ?? []).map((m) => (
                  <TableRow key={`${m.provider}/${m.id}`}>
                    <TableCell>
                      <p className="font-medium">{m.id}</p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {m.provider}
                      </p>
                    </TableCell>
                    <TableCell>
                      <p className="text-xs">{m.claim}</p>
                      <p className="mt-1 max-w-80 break-all whitespace-normal font-mono text-xs">
                        {m.value}
                      </p>
                    </TableCell>
                    <TableCell className="capitalize">{m.role}</TableCell>
                    <TableCell>{m.claimsMaxAgeMinutes} minutes</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {!directory.ssoMappings?.length ? (
              <p className="empty-state">
                No SSO mappings configured for this workspace.
              </p>
            ) : null}
            <h3 className="text-sm font-medium">Active mapped users</h3>
            <p className="text-xs leading-5 text-muted-foreground">
              Claim snapshots expire and require sign-in again. Upstream changes
              are observed at the next sign-in. Group-overage tokens receive no
              group grants; app-role mappings remain available.
            </p>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Person / provider</TableHead>
                  <TableHead>SSO / effective role</TableHead>
                  <TableHead>Matching mappings</TableHead>
                  <TableHead>Claims expire</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(directory.ssoMembers ?? []).map((m) => (
                  <TableRow key={m.user.id}>
                    <TableCell>
                      <p className="font-medium">{m.user.name}</p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {m.user.provider}
                      </p>
                    </TableCell>
                    <TableCell className="capitalize">
                      {m.mappedRole} / {m.effectiveRole}
                    </TableCell>
                    <TableCell>
                      {m.mappingIds.map((id) => (
                        <p
                          key={id}
                          className="max-w-64 break-all whitespace-normal font-mono text-xs"
                        >
                          {id}
                        </p>
                      ))}
                    </TableCell>
                    <TableCell className="text-xs">
                      {new Date(m.expiresAt).toLocaleString()}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {!directory.ssoMembers?.length ? (
              <p className="empty-state">
                No active mapped sign-ins in this view.
              </p>
            ) : null}
            {directory.ssoMembersTruncated ? (
              <p className="text-xs text-muted-foreground">
                Showing at most 200 active mapped users. This is a bounded view,
                not the full upstream directory.
              </p>
            ) : null}
          </TabsContent>
          <TabsContent value="audit" className="space-y-4">
            <div className="resource-toolbar">
              <Input
                className="max-w-sm"
                aria-label="Search loaded audit events"
                placeholder="Search loaded audit events"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
              <span className="text-xs text-muted-foreground">
                {audit.items.length} loaded · retained for 90 days
              </span>
            </div>
            <div className="max-h-[480px] overflow-auto rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Time</TableHead>
                    <TableHead>Actor</TableHead>
                    <TableHead>Action / resource</TableHead>
                    <TableHead>Outcome</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((e) => (
                    <TableRow key={e.id}>
                      <TableCell className="align-top text-xs">
                        {new Date(e.at).toLocaleString()}
                      </TableCell>
                      <TableCell className="align-top">
                        <p className="text-sm">{e.actorName}</p>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {e.actorKind}
                        </p>
                        <p
                          className="mt-1 max-w-40 truncate font-mono text-xs"
                          title={e.actorId}
                        >
                          {e.actorId}
                        </p>
                      </TableCell>
                      <TableCell className="align-top">
                        <p className="text-xs">{e.action}</p>
                        <p className="mt-1 max-w-72 break-all font-mono text-xs">
                          {e.resource}
                        </p>
                        {e.revision ? (
                          <p className="mt-1 text-xs">Revision {e.revision}</p>
                        ) : null}
                        <p
                          className="mt-1 max-w-72 truncate text-xs text-muted-foreground"
                          title={e.requestId}
                        >
                          {e.requestId}
                        </p>
                      </TableCell>
                      <TableCell className="align-top text-xs">
                        {e.outcome}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            {!rows.length ? (
              <p className="empty-state">
                No matching events in the loaded history.
              </p>
            ) : null}
            {audit.nextCursor ? (
              <Button
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={older}
              >
                Load older events
              </Button>
            ) : null}
          </TabsContent>
        </Tabs>
      </CardContent>
      <Dialog
        open={!!draft}
        onOpenChange={(open) => {
          if (!open && !busy) setDraft(null);
        }}
      >
        <DialogContent className="max-h-[90svh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              {draft?.version ? "Edit" : "Add"}{" "}
              {draft?.kind === "account" ? "service account" : draft?.kind}
            </DialogTitle>
            <DialogDescription>
              Changes apply only to workspace {scope.tenant}. Membership changes
              take effect on subsequent requests.
            </DialogDescription>
          </DialogHeader>
          {draft ? (
            <form
              className="space-y-4"
              onSubmit={(e) => {
                e.preventDefault();
                void save();
              }}
            >
              {draft.kind === "member" ? (
                <>
                  <div>
                    <Label htmlFor="member-provider">Identity provider</Label>
                    <select
                      id="member-provider"
                      className={field}
                      disabled={draft.version > 0}
                      value={draft.provider}
                      onChange={(e) =>
                        setDraft({ ...draft, provider: e.target.value })
                      }
                    >
                      {directory.providers.map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.name} ({p.type})
                        </option>
                      ))}
                    </select>
                  </div>
                  <div>
                    <Label htmlFor="member-subject">Identity subject</Label>
                    <Input
                      id="member-subject"
                      required
                      maxLength={500}
                      className="mt-2"
                      disabled={draft.version > 0}
                      value={draft.subject}
                      onChange={(e) =>
                        setDraft({ ...draft, subject: e.target.value })
                      }
                    />
                    <p className="mt-2 text-xs text-muted-foreground">
                      Use the exact OIDC subject or immutable LDAP identifier,
                      not an email address or a username. A person without
                      access can see their identity subject after signing in.
                    </p>
                  </div>
                </>
              ) : (
                <>
                  <div>
                    <Label htmlFor="access-id">Identifier</Label>
                    <Input
                      id="access-id"
                      required
                      pattern="[a-z0-9][a-z0-9-]{0,62}"
                      className="mt-2"
                      disabled={draft.version > 0}
                      value={draft.id}
                      onChange={(e) =>
                        setDraft({ ...draft, id: e.target.value })
                      }
                    />
                  </div>
                  <div>
                    <Label htmlFor="access-name">Display name</Label>
                    <Input
                      id="access-name"
                      required
                      maxLength={200}
                      className="mt-2"
                      value={draft.name}
                      onChange={(e) =>
                        setDraft({ ...draft, name: e.target.value })
                      }
                    />
                  </div>
                </>
              )}
              <div>
                <Label htmlFor="access-role">Role</Label>
                <select
                  id="access-role"
                  className={field}
                  value={draft.role}
                  onChange={(e) =>
                    setDraft({ ...draft, role: e.target.value as Role })
                  }
                >
                  {(draft.kind === "member"
                    ? ["viewer", "agent", "editor", "owner"]
                    : ["viewer", "agent", "editor"]
                  ).map((r) => (
                    <option value={r} key={r}>
                      {r}
                    </option>
                  ))}
                </select>
              </div>
              {draft.kind === "team" ? (
                <fieldset>
                  <legend className="mb-2 text-sm font-medium">
                    Workspace members
                  </legend>
                  <div className="max-h-48 space-y-2 overflow-auto rounded-md border p-3">
                    {directory.members.map((m) => (
                      <label
                        key={m.userId}
                        className="flex items-center gap-2 text-sm"
                      >
                        <input
                          type="checkbox"
                          checked={draft.members.includes(m.userId)}
                          onChange={(e) =>
                            setDraft({
                              ...draft,
                              members: e.target.checked
                                ? [...draft.members, m.userId]
                                : draft.members.filter((id) => id !== m.userId),
                            })
                          }
                        />
                        {m.user.name}
                      </label>
                    ))}
                    {!directory.members.length ? (
                      <p className="text-xs text-muted-foreground">
                        Add workspace members first.
                      </p>
                    ) : null}
                  </div>
                </fieldset>
              ) : null}
              {draft.kind === "account" ? (
                <div>
                  <Label htmlFor="account-expiry">Expires (local time)</Label>
                  <Input
                    id="account-expiry"
                    type="datetime-local"
                    required
                    className="mt-2"
                    value={draft.expires}
                    onChange={(e) =>
                      setDraft({ ...draft, expires: e.target.value })
                    }
                  />
                </div>
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
                <Button type="submit" disabled={busy}>
                  {busy ? "Saving…" : "Save"}
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
            <DialogTitle>Remove access</DialogTitle>
            <DialogDescription>
              Remove the selected grant for {remove?.name} from {scope.tenant}?
              Independent SSO grants remain until their mapping or upstream
              assignment is removed. Revoked service-account tokens stop working
              and cannot be recovered. Member access can be granted again.
            </DialogDescription>
          </DialogHeader>
          {error ? (
            <p role="alert" className="text-sm text-destructive">
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
              onClick={confirmRemove}
            >
              {busy ? "Removing…" : "Remove access"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
      <Dialog
        open={!!secret}
        onOpenChange={(open) => {
          if (!open) setSecret("");
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Service-account token</DialogTitle>
            <DialogDescription>
              This token is shown once. Store it in your CLI or MCP secret
              configuration before closing this dialog.
            </DialogDescription>
          </DialogHeader>
          <Label htmlFor="account-token">Token</Label>
          <Input
            id="account-token"
            readOnly
            value={secret}
            className="font-mono text-xs"
            onFocus={(e) => e.target.select()}
          />
          <div className="dialog-actions">
            <Button onClick={() => setSecret("")}>Done</Button>
          </div>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
