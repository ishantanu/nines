import {
  PlatformChangeDialog,
  type PlatformDraft as Draft,
} from "./platform-change-dialog";
import { useEffect, useRef, useState } from "react";
import { Brand } from "./brand";
import { Button } from "./ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "./ui/card";
import { Badge } from "./ui/badge";

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "./ui/table";
import { PageHeader } from "./workspace-chrome";
import { followLink, workspacePath } from "@/lib/navigation";
import { message, type AuthInfo, type Tenant } from "@/lib/api";
import {
  platformRead,
  platformWrite,
  type PlatformOverview,
  type PlatformUser,
  type PlatformEvent,
  type PlatformPolicy,
} from "@/lib/platform";

const blank = (tenant: string, provider: string): Draft => ({
  action: "create",
  tenant,
  provider,
  username: "",
  name: "",
  email: "",
  password: "",
  role: "viewer",
  reason: "",
  enabled: false,
  limit: "1000",
  version: 0,
});
export function PlatformConsole({
  page,
  auth,
  tenants,
  onSignOut,
  onRefreshAccess,
}: {
  page: "tenants" | "users" | "audit";
  auth: AuthInfo;
  tenants: Tenant[];
  onSignOut: () => void;
  onRefreshAccess: () => void;
}) {
  const [overview, setOverview] = useState<PlatformOverview>();
  const [users, setUsers] = useState<PlatformUser[]>([]),
    [events, setEvents] = useState<PlatformEvent[]>([]);
  const [next, setNext] = useState(""),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [epoch, setEpoch] = useState(0);
  const [draft, setDraft] = useState<Draft>();
  const live = useRef(true),
    generation = useRef(0);
  const csrf = auth.csrfToken ?? "";
  useEffect(() => {
    document.title = `${page === "tenants" ? "Tenants" : page === "users" ? "Users" : "Audit history"} | Platform administration | Nines`;
  }, [page]);
  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
      generation.current++;
    };
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    const current = ++generation.current;
    setLoading(true);
    setError("");
    setUsers([]);
    setEvents([]);
    setNext("");
    setDraft(undefined);
    void Promise.all([
      platformRead<PlatformOverview>("overview", controller.signal),
      page === "users"
        ? platformRead<{ items: PlatformUser[]; nextCursor: string }>(
            "users",
            controller.signal,
          )
        : page === "audit"
          ? platformRead<{ items: PlatformEvent[]; nextCursor: string }>(
              "audit",
              controller.signal,
            )
          : Promise.resolve(null),
    ])
      .then(([info, result]) => {
        if (controller.signal.aborted || current !== generation.current) return;
        setOverview(info);
        if (result) {
          if (page === "users") setUsers(result.items as PlatformUser[]);
          else setEvents(result.items as PlatformEvent[]);
          setNext(result.nextCursor);
        }
      })
      .catch((e) => {
        if (!controller.signal.aborted && current === generation.current) {
          setOverview(undefined);
          setError(message(e));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted && current === generation.current)
          setLoading(false);
      });
    return () => controller.abort();
  }, [page, epoch]);
  const base = () =>
    blank(
      overview?.tenants[0]?.id ?? "",
      overview?.providers.find((p) => p.type === "local")?.id ?? "",
    );
  function editUser(action: Draft["action"], user: PlatformUser) {
    setDraft({
      ...base(),
      action,
      user,
      provider: user.user.provider,
      username: user.user.subject,
      name: user.user.name,
      email: user.user.email ?? "",
      tenant: user.memberships[0]?.tenant ?? overview?.tenants[0]?.id ?? "",
      role: user.memberships[0]?.member.role ?? "viewer",
      version: user.user.version,
    });
  }
  async function loadMore() {
    if (!next || busy) return;
    const current = generation.current;
    setBusy(true);
    setError("");
    const controller = new AbortController();
    try {
      if (page === "users") {
        const r = await platformRead<{
          items: PlatformUser[];
          nextCursor: string;
        }>("users?before=" + encodeURIComponent(next), controller.signal);
        if (live.current && current === generation.current) {
          setUsers((old) => [...old, ...r.items]);
          setNext(r.nextCursor);
        }
      } else {
        const r = await platformRead<{
          items: PlatformEvent[];
          nextCursor: string;
        }>("audit?before=" + encodeURIComponent(next), controller.signal);
        if (live.current && current === generation.current) {
          setEvents((old) => [...old, ...r.items]);
          setNext(r.nextCursor);
        }
      }
    } catch (e) {
      if (live.current && current === generation.current) setError(message(e));
    } finally {
      if (live.current) setBusy(false);
    }
  }
  async function editPolicy(tenant: string) {
    if (busy) return;
    const current = generation.current;
    setBusy(true);
    setError("");
    const controller = new AbortController();
    try {
      const r = await platformRead<PlatformPolicy>(
        "tenants/" + encodeURIComponent(tenant) + "/system-one",
        controller.signal,
      );
      if (live.current && current === generation.current)
        setDraft({
          ...base(),
          action: "policy",
          tenant,
          enabled: r.policy.enabled && r.configured,
          limit: String(r.policy.dailyRequestLimit),
          version: r.policy.version,
          name: `${r.reservedRequests} requests reserved on ${r.date} (UTC). ${r.route?.model ?? "No server route configured"}.`,
        });
    } catch (e) {
      if (live.current && current === generation.current) setError(message(e));
    } finally {
      if (live.current) setBusy(false);
    }
  }
  async function save() {
    if (!draft || busy) return;
    const current = generation.current;
    setBusy(true);
    setError("");
    const { action, reason, tenant } = draft;
    let path = "",
      body: unknown;
    if (action === "create") {
      path = "users/create";
      body = {
        reason,
        tenant,
        user: {
          provider: draft.provider,
          username: draft.username,
          name: draft.name,
          email: draft.email,
          password: draft.password,
          role: draft.role,
          expectedVersion: 0,
        },
      };
    } else if (action === "profile") {
      path = "users/update";
      body = {
        reason,
        tenant,
        userId: draft.user!.user.id,
        user: {
          name: draft.name,
          email: draft.email,
          password: draft.password,
          expectedVersion: draft.version,
        },
      };
    } else if (action === "revoke") {
      path = "users/revoke-sessions";
      body = {
        reason,
        userId: draft.user!.user.id,
        expectedVersion: draft.version,
      };
    } else if (action === "membership") {
      const existing = draft.user?.memberships.find((m) => m.tenant === tenant);
      path = "memberships/save";
      body = {
        reason,
        tenant,
        provider: draft.provider,
        subject: draft.username,
        role: draft.role,
        expectedVersion: existing?.member.version ?? 0,
      };
    } else if (action === "remove") {
      path = "memberships/remove";
      body = {
        reason,
        tenant,
        userId: draft.user!.user.id,
        expectedVersion: draft.user!.memberships.find(
          (m) => m.tenant === tenant,
        )!.member.version,
      };
    } else {
      path = "system-one/save";
      body = {
        reason,
        tenant,
        policy: {
          version: draft.version,
          enabled: draft.enabled,
          dailyRequestLimit: Number(draft.limit),
        },
      };
    }
    // Passwords leave component state as soon as a request is submitted.
    setDraft((old) => (old ? { ...old, password: "" } : old));
    try {
      await platformWrite(path, csrf, body);
      if (live.current && current === generation.current) {
        setDraft(undefined);
        setEpoch((old) => old + 1);
        onRefreshAccess();
      }
    } catch (e) {
      if (live.current && current === generation.current) setError(message(e));
    } finally {
      if (live.current) setBusy(false);
    }
  }
  const set = (value: Partial<Draft>) =>
    setDraft((old) => (old ? { ...old, ...value } : old));
  const titles = { tenants: "Tenants", users: "Users", audit: "Audit history" };
  return (
    <div className="min-h-screen bg-background" data-replay-block data-private>
      <header className="flex min-h-16 flex-wrap items-center justify-between gap-3 border-b px-4 py-3 sm:px-6">
        <div className="flex items-center gap-4">
          <Brand className="h-7" />
          <span className="text-sm font-medium">Platform administration</span>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-xs text-muted-foreground">
            {auth.actor?.name}
          </span>
          {tenants[0] ? (
            <a
              className="text-sm underline"
              href={workspacePath(tenants[0].id)}
              onClick={(e) => followLink(e, workspacePath(tenants[0].id))}
            >
              Open workspace
            </a>
          ) : null}
          <Button variant="outline" size="sm" onClick={onSignOut}>
            Sign out
          </Button>
        </div>
      </header>
      <div className="mx-auto grid max-w-screen-2xl gap-6 p-4 sm:p-6 lg:grid-cols-[200px_minmax(0,1fr)]">
        <nav
          aria-label="Platform navigation"
          className="flex flex-wrap gap-1 lg:flex-col"
        >
          {(["tenants", "users", "audit"] as const).map((item) => (
            <a
              key={item}
              href={"/admin/" + item}
              aria-current={page === item ? "page" : undefined}
              onClick={(e) => followLink(e, "/admin/" + item)}
              className={`rounded-md px-3 py-2 text-sm ${page === item ? "bg-muted font-medium" : "hover:bg-muted"}`}
            >
              {titles[item]}
            </a>
          ))}
        </nav>
        <main className="min-w-0 space-y-5">
          <PageHeader
            title={titles[page]}
            description="Platform controls are separate from tenant access. Administrative changes require a reason and are audited."
          >
            <Button
              variant="outline"
              disabled={busy || loading}
              onClick={() => setEpoch((n) => n + 1)}
            >
              Refresh
            </Button>
            {page === "users" &&
            overview?.providers.some((p) => p.type === "local") ? (
              <Button
                disabled={busy || loading}
                onClick={() => setDraft(base())}
              >
                Create user
              </Button>
            ) : null}
          </PageHeader>
          {error ? (
            <p
              role="alert"
              className="rounded-md border border-destructive p-3 text-sm"
            >
              {error}
            </p>
          ) : null}
          {loading ? <p role="status">Loading administration…</p> : null}
          {!loading && overview && page === "tenants" ? (
            <>
              <Card>
                <CardHeader>
                  <CardTitle>
                    <h2>Configured tenants</h2>
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <p className="mb-4 text-sm text-muted-foreground">
                    Tenant provisioning, provider credentials and pinned model
                    routes are managed in server configuration. Platform
                    administration does not grant access to a tenant’s SLO data.
                  </p>
                  <div className="overflow-x-auto">
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>Tenant</TableHead>
                          <TableHead>Metrics provider</TableHead>
                          <TableHead>Connections</TableHead>
                          <TableHead>System One route</TableHead>
                          <TableHead>Actions</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {overview.tenants.map((tenant) => (
                          <TableRow key={tenant.id}>
                            <TableCell>
                              <span className="font-medium">{tenant.name}</span>
                              <div className="text-xs text-muted-foreground">
                                {tenant.id}
                              </div>
                            </TableCell>
                            <TableCell>
                              {tenant.sourceKind || "manual"}
                            </TableCell>
                            <TableCell>{tenant.connections}</TableCell>
                            <TableCell>
                              {tenant.systemOne?.model ?? "Not configured"}
                            </TableCell>
                            <TableCell>
                              <Button
                                size="sm"
                                variant="outline"
                                disabled={busy}
                                onClick={() => void editPolicy(tenant.id)}
                              >
                                Jev policy
                              </Button>
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </div>
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardTitle>
                    <h2>Platform administrators</h2>
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  <p className="mb-3 text-sm text-muted-foreground">
                    Assigned by exact provider and identity subject in the
                    authentication configuration. Tenant roles and SSO tenant
                    mappings cannot grant this access.
                  </p>
                  <ul className="space-y-2 text-sm">
                    {overview.administrators.map((a) => (
                      <li key={a.provider + ":" + a.subject}>
                        {a.provider} ·{" "}
                        <span className="break-all font-mono text-xs">
                          {a.subject}
                        </span>
                      </li>
                    ))}
                  </ul>
                </CardContent>
              </Card>
            </>
          ) : null}
          {!loading && overview && page === "users" ? (
            <Card>
              <CardContent className="pt-5">
                <div className="overflow-x-auto">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>User</TableHead>
                        <TableHead>Identity</TableHead>
                        <TableHead>Tenant access</TableHead>
                        <TableHead>Actions</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {users.map((user) => (
                        <TableRow key={user.user.id}>
                          <TableCell>
                            <span className="font-medium">
                              {user.user.name}
                            </span>
                            {user.platformAdmin ? (
                              <Badge variant="secondary" className="ml-2">
                                Platform admin
                              </Badge>
                            ) : null}
                            <div className="text-xs text-muted-foreground">
                              {user.user.email}
                            </div>
                          </TableCell>
                          <TableCell>
                            <div>{user.user.provider}</div>
                            <div className="break-all font-mono text-xs">
                              {user.user.subject}
                            </div>
                          </TableCell>
                          <TableCell>
                            {Object.entries(user.effectiveGrants).map(
                              ([tenant, role]) => (
                                <div key={tenant}>
                                  {tenant}: {role}
                                </div>
                              ),
                            )}
                            {Object.keys(user.effectiveGrants).length === 0 ? (
                              <span className="text-muted-foreground">
                                No tenant access
                              </span>
                            ) : null}
                          </TableCell>
                          <TableCell>
                            <div className="flex max-w-xs flex-wrap gap-2">
                              {user.local ? (
                                <Button
                                  size="sm"
                                  variant="outline"
                                  disabled={busy}
                                  onClick={() => editUser("profile", user)}
                                >
                                  Edit account
                                </Button>
                              ) : null}
                              <Button
                                size="sm"
                                variant="outline"
                                disabled={busy}
                                onClick={() => editUser("membership", user)}
                              >
                                Tenant access
                              </Button>
                              {user.memberships.length ? (
                                <Button
                                  size="sm"
                                  variant="outline"
                                  disabled={busy}
                                  onClick={() => editUser("remove", user)}
                                >
                                  Remove access
                                </Button>
                              ) : null}
                              <Button
                                size="sm"
                                variant="outline"
                                disabled={busy}
                                onClick={() => editUser("revoke", user)}
                              >
                                Revoke sessions
                              </Button>
                            </div>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
                {users.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    No identities found.
                  </p>
                ) : null}
              </CardContent>
            </Card>
          ) : null}
          {!loading && overview && page === "audit" ? (
            <Card>
              <CardContent className="space-y-4 pt-5">
                <p className="text-sm text-muted-foreground">
                  Platform changes are retained for{" "}
                  {overview.auditRetentionDays} days. Passwords, session tokens
                  and API keys are excluded.
                </p>
                {events.map((event) => (
                  <article
                    key={event.id}
                    className="space-y-1 rounded-md border p-3 text-sm"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <span className="font-medium">{event.action}</span>
                      <span>{event.outcome}</span>
                    </div>
                    <p className="break-all text-xs text-muted-foreground">
                      {new Date(event.at).toLocaleString()} ·{" "}
                      {event.actorName ?? event.actorId}
                      {event.actorProvider
                        ? ` (${event.actorProvider})`
                        : ""} · {event.resource}
                    </p>
                    <p>{event.reason}</p>
                    <details>
                      <summary className="cursor-pointer text-xs">
                        Change details
                      </summary>
                      <pre className="mt-2 overflow-auto whitespace-pre-wrap break-all text-xs">
                        {JSON.stringify(
                          {
                            id: event.id,
                            actorId: event.actorId,
                            details: event.details,
                          },
                          null,
                          2,
                        )}
                      </pre>
                    </details>
                  </article>
                ))}
                {events.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    No platform changes recorded.
                  </p>
                ) : null}
              </CardContent>
            </Card>
          ) : null}
          {next ? (
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => void loadMore()}
            >
              Load more
            </Button>
          ) : null}
        </main>
      </div>
      {draft ? (
        <PlatformChangeDialog
          draft={draft}
          overview={overview}
          busy={busy}
          csrf={csrf}
          onChange={set}
          onClose={() => setDraft(undefined)}
          onSave={save}
        />
      ) : null}
    </div>
  );
}
