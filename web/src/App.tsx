import { ReplayLabel } from "@/components/replay-label";
import { Brand } from "@/components/brand";
import {
  lazy,
  Suspense,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  useMemo,
  useSyncExternalStore,
} from "react";
import {
  Activity,
  Video,
  MessageSquare,
  Moon,
  House,
  Database,
  KeyRound,
  Terminal,
  Bot,
  Search,
  Settings2,
  ShieldCheck,
  Waypoints,
  Workflow,
  Network,
  Sun,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  loadData,
  healthOf,
  date,
  message,
  discoverTenants,
  createTenantSession,
  authSession,
  authAction,
  canEdit,
  type AuthInfo,
  type Data,
  type Tenant,
  type TenantSession,
  type RequestScope,
} from "@/lib/api";
import {
  WorkspaceHeader,
  PageHeader,
  WorkspaceFooter,
} from "@/components/workspace-chrome";

import { Status } from "@/components/status";
import { ManifestEditor } from "@/components/manifest-editor";
import { initialDarkMode, savePreference } from "@/lib/preferences";
import type { ReplayRef } from "@/lib/replays";

import { MaskedReplayCapture } from "@/lib/replay-capture";
import type { ReplayOption } from "@/components/session-replays";
import { DemoLogin } from "@/components/demo-login";
import {
  isSessionReplayConfigured,
  startDemoSessionReplay,
} from "@/lib/session-replay";

import { ObjectiveDetail, Objectives } from "@/components/objectives";
import { cn } from "@/lib/utils";
import {
  useRoute,
  workspacePath,
  navigateTo,
  followLink,
  loginReturnPath,
  clearLoginReturn,
  showLogin,
  parseRoute,
  routeFromLocation,
  type Page,
} from "@/lib/navigation";
const JourneyDetail = lazy(() =>
  import("@/components/journey-detail").then((m) => ({
    default: m.JourneyDetail,
  })),
);
const Login = lazy(() =>
  import("@/components/login").then((m) => ({ default: m.Login })),
);
const ObjectiveCreate = lazy(() =>
  import("@/components/objective-create").then((module) => ({
    default: module.ObjectiveCreate,
  })),
);
const Workflows = lazy(() =>
  import("@/components/workflows").then((module) => ({
    default: module.Workflows,
  })),
);
const Home = lazy(() =>
  import("@/components/home").then((m) => ({ default: m.Home })),
);
const AccessControl = lazy(() =>
  import("@/components/administration").then((m) => ({
    default: m.AccessControl,
  })),
);
const Connections = lazy(() =>
  import("@/components/administration").then((m) => ({
    default: m.Connections,
  })),
);
const Clients = lazy(() =>
  import("@/components/administration").then((m) => ({ default: m.Clients })),
);
const DependencyMaps = lazy(() =>
  import("@/components/dependency-maps").then((m) => ({
    default: m.DependencyMaps,
  })),
);
const MeasurementBindings = lazy(() =>
  import("@/components/measurement-bindings").then((m) => ({
    default: m.MeasurementBindings,
  })),
);
const Investigations = lazy(() =>
  import("./components/investigations").then((m) => ({
    default: m.Investigations,
  })),
);
const IntentAuthoring = lazy(() =>
  import("@/components/intent-workspace").then((m) => ({
    default: m.IntentWorkspace,
  })),
);
const SessionReplays = lazy(() =>
  import("./components/session-replays").then((m) => ({
    default: m.SessionReplays,
  })),
);
const AuditLog = lazy(() =>
  import("./components/audit-log").then((m) => ({ default: m.AuditLog })),
);
const navigation = [
  { id: "home" as const, label: "Home", icon: House },
  {
    id: "investigations" as const,
    label: "Reliability reviews",
    icon: MessageSquare,
  },
  { id: "journeys" as const, label: "Critical user journeys", icon: Waypoints },
  {
    id: "objectives" as const,
    label: "Service level objectives",
    icon: Activity,
  },
  { id: "intents" as const, label: "SLO intents", icon: Activity },
  { id: "maps" as const, label: "Service dependencies", icon: Network },

  { id: "replays" as const, label: "Session replay", icon: Video },
  { id: "agents" as const, label: "Agent evidence", icon: Bot },
  { id: "workflows" as const, label: "Workflows", icon: Workflow },
  { id: "alerts" as const, label: "Alert activity", icon: ShieldCheck },
  { id: "connections" as const, label: "Data sources", icon: Database },
  { id: "audit" as const, label: "Audit log", icon: ShieldCheck },
  { id: "access" as const, label: "Access control", icon: KeyRound },
  { id: "clients" as const, label: "API and clients", icon: Terminal },
];
const copy = {
  audit: [
    "Audit log",
    "Search decisions, review reasons and exact proposal snapshots for this tenant.",
  ],
  replays: [
    "Session replay",
    "Play masked sessions and link observed behavior to journeys, objectives and violations.",
  ],
  investigations: [
    "Reliability reviews",
    "Record reliability questions, findings and revision-bound evidence.",
  ],
  intents: [
    "SLO intents",
    "Turn a reliability goal into a measured, reviewable SLO and journey link.",
  ],
  maps: [
    "Service dependencies",
    "Inspect service relationships, linked objectives and journey impact.",
  ],
  bindings: [
    "Measurements",
    "Define eligible events and success criteria for SLOs.",
  ],
  home: ["Overview", "Tenant reliability, journey impact and configuration."],
  connections: [
    "Data sources",
    "Manage metrics connections and recording-rule destinations.",
  ],
  access: [
    "Access control",
    "Tenant roles, effective permissions and configured credential grants.",
  ],
  clients: [
    "API and clients",
    "Create and manage versioned resources from applications and automation.",
  ],
  workflows: [
    "Workflows",
    "Reliability reviews, change proposals and reusable procedures.",
  ],
  journeys: [
    "Critical user journeys",
    "Manage journey definitions, dependencies and linked objectives.",
  ],
  objectives: [
    "Service level objectives",
    "Monitor reliability, error budgets and objective coverage.",
  ],
  agents: [
    "Agent evidence",
    "Check reported workflow runs against journey requirements.",
  ],
  alerts: [
    "Alert activity",
    "Review burn-rate transitions and notification delivery.",
  ],
};

function Credentials({
  open,
  onOpenChange,
  onConnect,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onConnect: (token: string) => void;
}) {
  const [token, setToken] = useState("");
  return (
    <Dialog
      open={open}
      onOpenChange={(value) => {
        if (!value) setToken("");
        onOpenChange(value);
      }}
    >
      <DialogContent data-replay-block data-private>
        <DialogHeader>
          <DialogTitle>Connect to this workspace</DialogTitle>
          <DialogDescription>
            Enter your workspace API token. It is retained for this tab session.
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            onConnect(token);
            onOpenChange(false);
            setToken("");
          }}
        >
          <Label htmlFor="api-token">API token</Label>
          <Input
            id="api-token"
            className="mt-2"
            type="password"
            autoComplete="off"
            value={token}
            onChange={(e) => setToken(e.target.value)}
          />
          <div className="dialog-actions">
            <Button type="submit">Connect</Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
export default function App() {
  const route = useRoute();
  const [token, setToken] = useState("");
  const [demoAuthenticated, setDemoAuthenticated] = useState(false);
  const [demoReplayConsent, setDemoReplayConsent] = useState(false);
  const [darkMode, setDarkMode] = useState(initialDarkMode);
  const [connection, setConnection] = useState(0);
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [session, setSession] = useState<{
    scope: TenantSession;
    key: number;
  } | null>(null);
  const [credentials, setCredentials] = useState(false);
  const [auth, setAuth] = useState<AuthInfo | null>(null);
  const chosenTenant = useRef("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const active = useRef<TenantSession | null>(null);
  const generation = useRef(0);
  useLayoutEffect(() => {
    document.documentElement.classList.toggle("dark", darkMode);
    document.documentElement.style.colorScheme = darkMode ? "dark" : "light";
    savePreference("nines-theme", darkMode ? "dark" : "light");
  }, [darkMode]);
  function chooseTenant(id: string, fromHistory = false) {
    const tenant = tenants.find((t) => t.id === id);
    if (!tenant) return;
    if (!fromHistory)
      navigateTo(
        workspacePath(id, route.kind === "workspace" ? route.page : "home"),
      );
    active.current?.close();
    chosenTenant.current = id;
    const scope = createTenantSession(
      token,
      id,
      tenant.role,
      token ? undefined : auth?.csrfToken,
    );
    active.current = scope;
    setSession({ scope, key: ++generation.current });
  }
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError("");
    active.current?.close();
    setSession(null);
    setTenants([]);
    authSession(controller.signal)
      .then(async (info) => {
        if (controller.signal.aborted) return;
        setAuth(info);
        if (info.enabled && !info.authenticated && !token) {
          showLogin();
          return;
        }
        const allowed = await discoverTenants(token, controller.signal);
        if (controller.signal.aborted) return;
        setTenants(allowed);
        if (!allowed.length)
          throw new Error(
            "You have no workspace memberships. Ask a workspace owner to grant access using the identity below.",
          );
        const current = routeFromLocation(
          window.location.pathname +
            window.location.search +
            window.location.hash,
        );
        const requested =
          current.kind === "workspace"
            ? current
            : parseRoute(loginReturnPath() ?? "/");
        const tenant =
          requested.kind === "workspace"
            ? allowed.find((t) => t.id === requested.tenant)
            : (allowed.find((t) => t.id === chosenTenant.current) ??
              allowed[0]);
        if (!tenant)
          throw new Error(
            "You do not have access to the tenant in this link. Choose an authorized workspace below.",
          );
        if (current.kind === "root" || current.kind === "login") {
          navigateTo(
            requested.kind === "workspace"
              ? workspacePath(requested.tenant, requested.page, requested.id)
              : workspacePath(tenant.id),
            true,
          );
          clearLoginReturn();
        } else if (current.kind === "workspace") {
          navigateTo(
            workspacePath(current.tenant, current.page, current.id),
            true,
          );
        }
        chosenTenant.current = tenant.id;
        const scope = createTenantSession(
          token,
          tenant.id,
          tenant.role,
          token ? undefined : info.csrfToken,
        );
        active.current = scope;
        setSession({ scope, key: ++generation.current });
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted) {
          if (e instanceof Error && "status" in e && e.status === 401)
            showLogin();
          setError(message(e));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => {
      controller.abort();
      active.current?.close();
    };
  }, [token, connection]);
  useEffect(() => {
    if (
      loading ||
      route.kind !== "workspace" ||
      !session ||
      (session.scope.tenant === route.tenant && !session.scope.signal.aborted)
    )
      return;
    active.current?.close();
    if (tenants.some((t) => t.id === route.tenant))
      chooseTenant(route.tenant, true);
  }, [route, loading, session, tenants]);
  useEffect(() => {
    if (
      route.kind !== "not-found" &&
      auth?.enabled &&
      !auth.authenticated &&
      !token
    ) {
      showLogin();
      document.title = "Sign in | Nines";
    }
  }, [route, auth?.enabled, auth?.authenticated, token]);
  useEffect(() => {
    let pending: ReturnType<typeof setTimeout> | undefined;
    function refreshAccess() {
      if (pending) return;
      pending = setTimeout(() => {
        pending = undefined;
        setConnection((n) => n + 1);
      }, 0);
    }
    window.addEventListener("nines-access-changed", refreshAccess);
    return () => {
      window.removeEventListener("nines-access-changed", refreshAccess);
      clearTimeout(pending);
    };
  }, []);
  useEffect(() => {
    if (!auth?.enabled || !auth.authenticated || token) return;
    const controller = new AbortController();
    const timer = setInterval(() => {
      Promise.all([
        authSession(controller.signal),
        discoverTenants("", controller.signal),
      ])
        .then(([info, allowed]) => {
          if (controller.signal.aborted) return;
          if (
            !info.authenticated ||
            info.actor?.id !== auth.actor?.id ||
            info.csrfToken !== auth.csrfToken ||
            JSON.stringify(allowed) !== JSON.stringify(tenants)
          ) {
            active.current?.close();
            setSession(null);
            setConnection((n) => n + 1);
          }
        })
        .catch((e: unknown) => {
          if (
            !controller.signal.aborted &&
            e instanceof Error &&
            "status" in e &&
            e.status === 401
          ) {
            active.current?.close();
            setSession(null);
            setConnection((n) => n + 1);
          }
        });
    }, 15000);
    return () => {
      clearInterval(timer);
      controller.abort();
    };
  }, [
    auth?.enabled,
    auth?.authenticated,
    auth?.actor?.id,
    auth?.csrfToken,
    token,
    tenants,
  ]);
  async function signOut() {
    try {
      if (auth?.authenticated)
        await authAction("/auth/logout", auth.csrfToken ?? "");
      active.current?.close();
      setSession(null);
      setTenants([]);
      setToken("");
      clearLoginReturn();
      navigateTo("/login", true);
      setConnection((n) => n + 1);
    } catch (e) {
      setError(message(e));
    }
  }
  return (
    <>
      {!session ? (
        <Button
          variant="ghost"
          size="icon"
          className="fixed right-4 top-4 z-50"
          onClick={() => setDarkMode((current) => !current)}
          aria-label={darkMode ? "Switch to light mode" : "Switch to dark mode"}
          aria-pressed={darkMode}
          title={darkMode ? "Switch to light mode" : "Switch to dark mode"}
        >
          {darkMode ? (
            <Sun className="size-4" aria-hidden="true" />
          ) : (
            <Moon className="size-4" aria-hidden="true" />
          )}
        </Button>
      ) : null}
      {route.kind === "not-found" ? (
        <main className="empty-state">
          <h1>Page not found</h1>
          <p>Check the address or return to your workspace.</p>
          <Button
            onClick={() =>
              navigateTo(
                session ? workspacePath(session.scope.tenant) : "/login",
                true,
              )
            }
          >
            Return to Nines
          </Button>
        </main>
      ) : session &&
        route.kind === "workspace" &&
        route.tenant !== session.scope.tenant ? (
        <main className="empty-state" role="status">
          <h1>Workspace access</h1>
          <p>
            {tenants.some((t) => t.id === route.tenant)
              ? "Switching workspace…"
              : "You do not have access to the tenant in this link."}
          </p>
          {tenants.map((t) => (
            <Button key={t.id} onClick={() => chooseTenant(t.id)}>
              {t.id}
            </Button>
          ))}
        </main>
      ) : session ? (
        <TenantWorkspace
          key={session.key}
          scope={session.scope}
          tenants={tenants}
          onTenant={chooseTenant}
          onCredentials={() => setCredentials(true)}
          credentialsOpen={credentials}
          identityLabel={token ? "API credential" : auth?.actor?.name}
          onSignOut={auth?.enabled || token ? signOut : undefined}
          darkMode={darkMode}
          onToggleTheme={() => setDarkMode((current) => !current)}
          demoAuthenticated={demoAuthenticated}
          demoReplayConsent={demoReplayConsent}
          onDemoSignIn={(allowReplay) => {
            const next = loginReturnPath();
            navigateTo(next ?? workspacePath(session.scope.tenant), true);
            clearLoginReturn();
            setDemoAuthenticated(true);
            setDemoReplayConsent(allowReplay);
          }}
          onToggleDemoReplay={() => {
            const allowReplay = !demoReplayConsent;
            setDemoReplayConsent(allowReplay);
          }}
          onDemoSignOut={() => {
            clearLoginReturn();
            navigateTo("/login", true);
            setDemoAuthenticated(false);
            setDemoReplayConsent(false);
          }}
        />
      ) : auth?.enabled && !auth.authenticated && !token && !loading ? (
        <Suspense
          fallback={
            <main className="empty-state" role="status">
              Loading sign-in…
            </main>
          }
        >
          <Login
            auth={auth}
            onSignedIn={() => setConnection((n) => n + 1)}
            onToken={() => setCredentials(true)}
          />
        </Suspense>
      ) : (
        <main className="flex min-h-screen items-center justify-center bg-background p-6">
          <Card className="w-full max-w-md">
            <CardContent className="space-y-4 pt-6">
              <Brand />
              <h1 className="text-xl font-semibold">
                {auth?.authenticated ? "Workspace access" : "Connect to Nines"}
              </h1>
              <p
                role={error ? "alert" : "status"}
                className="text-sm text-muted-foreground"
              >
                {error ||
                  (loading
                    ? "Discovering authorized tenants…"
                    : "Choose an API connection.")}
              </p>
              {auth?.authenticated ? (
                <div className="space-y-2 rounded-md border p-3 text-xs">
                  <p>Signed in as {auth.actor?.name}</p>
                  <p>Provider: {auth.actor?.provider}</p>
                  <p className="break-all">
                    Identity subject: {auth.actor?.identitySubject}
                  </p>
                  <Button variant="outline" size="sm" onClick={signOut}>
                    <ReplayLabel>Sign out</ReplayLabel>
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setConnection((n) => n + 1)}
                  >
                    Check access again
                  </Button>
                </div>
              ) : null}
              {tenants.map((t) => (
                <Button key={t.id} onClick={() => chooseTenant(t.id)}>
                  {t.id}
                </Button>
              ))}
              <Button onClick={() => setCredentials(true)}>
                Set API token
              </Button>
            </CardContent>
          </Card>
        </main>
      )}
      <Credentials
        open={credentials}
        onOpenChange={setCredentials}
        onConnect={(value) => {
          active.current?.close();
          setSession(null);
          setTenants([]);
          setToken(value);
          setConnection((n) => n + 1);
        }}
      />
    </>
  );
}
function TenantWorkspace({
  scope,
  tenants,
  onTenant,
  onCredentials,
  credentialsOpen,
  identityLabel,
  onSignOut,
  darkMode,
  onToggleTheme,
  demoAuthenticated,
  demoReplayConsent,
  onDemoSignIn,
  onToggleDemoReplay,
  onDemoSignOut,
}: {
  identityLabel?: string;
  onSignOut?: () => void;
  darkMode: boolean;
  onToggleTheme: () => void;
  demoAuthenticated: boolean;
  demoReplayConsent: boolean;
  onDemoSignIn: (allowReplay: boolean) => void;
  onToggleDemoReplay: () => void;
  onDemoSignOut: () => void;
  scope: RequestScope;
  tenants: Tenant[];
  onTenant: (id: string) => void;
  onCredentials: () => void;
  credentialsOpen: boolean;
}) {
  const route = useRoute();
  const page =
    route.kind === "workspace" && route.tenant === scope.tenant
      ? route.page
      : "home";
  const selected =
    route.kind === "workspace" && ["journeys", "agents"].includes(route.page)
      ? route.id
      : "";
  const slo =
    route.kind === "workspace" && route.page === "objectives" ? route.id : "";
  const setSLO = (id: string) =>
    navigateTo(workspacePath(scope.tenant, "objectives", id));
  const setSelected = (id: string) =>
    navigateTo(
      workspacePath(
        scope.tenant,
        page === "agents" ? "agents" : "journeys",
        id,
      ),
    );
  const [data, setData] = useState<Data | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [refresh, setRefresh] = useState(0);
  const [query, setQuery] = useState("");
  const [replayError, setReplayError] = useState("");
  const [replayFilter, setReplayFilter] = useState("all");
  const [updated, setUpdated] = useState<Date | null>(null);
  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    let controller: AbortController;
    setLoading(true);
    async function update() {
      controller = new AbortController();
      try {
        const result = await loadData(scope, controller.signal);
        if (active) {
          setData(result);
          setError("");
          setUpdated(new Date());
        }
      } catch (e) {
        if (active && !controller.signal.aborted) setError(message(e));
      } finally {
        if (active) {
          setLoading(false);
          timer = setTimeout(update, 15000);
        }
      }
    }
    void update();
    return () => {
      active = false;
      clearTimeout(timer);
      controller?.abort();
    };
  }, [scope, refresh]);
  const main = useRef<HTMLElement>(null);
  useEffect(() => {
    main.current?.focus({ preventScroll: true });
    document.title = `${slo || copy[page][0]} | Nines`;
  }, [page, slo, selected, route]);
  const activePage = slo ? "objectives" : page;
  const journeys = data?.journeys ?? [];
  const objectives = data?.objectives ?? [];
  const events = data?.events ?? [];
  const isDemo = data?.workspace.mode === "demo";
  const nativeCapture = useMemo(() => new MaskedReplayCapture(), [scope]);
  const nativeState = useSyncExternalStore(
    nativeCapture.subscribe,
    nativeCapture.getSnapshot,
  );
  useEffect(() => {
    const discard = () => nativeCapture.discard();
    scope.signal.addEventListener("abort", discard);
    return () => {
      scope.signal.removeEventListener("abort", discard);
      discard();
    };
  }, [nativeCapture, scope]);
  useEffect(() => {
    if (!data) return;
    if (isDemo && !demoAuthenticated) {
      showLogin();
      document.title = "Sign in | Nines";
    } else if (route.kind === "login" || route.kind === "root") {
      const next = loginReturnPath();
      const requested = next ? parseRoute(next) : null;
      navigateTo(
        requested?.kind === "workspace" && requested.tenant === scope.tenant
          ? next!
          : workspacePath(scope.tenant),
        true,
      );
      clearLoginReturn();
    }
  }, [data, isDemo, demoAuthenticated, route, scope.tenant]);

  useLayoutEffect(() => {
    if (isDemo && !demoAuthenticated) {
      nativeCapture.discard();
      return;
    }
    if (
      credentialsOpen ||
      ["access", "connections", "clients", "audit"].includes(page)
    ) {
      nativeCapture.pause();
    } else {
      void nativeCapture
        .resume()
        .catch(() => nativeCapture.stop("Recording could not resume."));
    }
  }, [nativeCapture, page, credentialsOpen, isDemo, demoAuthenticated]);
  const replayOptions: ReplayOption[] = [
    ...objectives.map((o) => ({
      label: `SLO: ${o.slo.name} · revision ${o.slo.revision ?? 1}`,
      ref: {
        kind: "ServiceLevelObjective",
        id: o.slo.id,
        revision: o.slo.revision ?? 1,
      },
    })),
    ...journeys.map((j) => ({
      label: `CUJ: ${j.resource.manifest.spec.displayName} · revision ${j.resource.revision}`,
      ref: {
        kind: "CriticalUserJourney",
        id: j.resource.manifest.metadata.name,
        revision: j.resource.revision,
      },
    })),
    ...events
      .filter((e) => e.event.kind === "firing")
      .map((e) => ({
        label: `Violation: ${e.event.sloId} · ${new Date(e.event.time).toLocaleString()}`,
        ref: {
          kind: "AlertEvent",
          id: e.event.id,
          revision: e.event.configRevision ?? 0,
        },
      })),
  ];
  const sensitivePage = ["access", "connections", "clients", "audit"].includes(
    page,
  );
  useLayoutEffect(() => {
    if (
      !isDemo ||
      !demoAuthenticated ||
      !demoReplayConsent ||
      credentialsOpen ||
      sensitivePage
    ) {
      return;
    }
    const replayAbort = new AbortController();
    let cancelled = false;
    let stopRecording: (() => void) | undefined;
    void startDemoSessionReplay(replayAbort.signal)
      .then((stop) => {
        if (cancelled) {
          stop();
          return;
        }
        stopRecording = stop;
        setReplayError("");
      })
      .catch((cause: unknown) => {
        if (!cancelled) {
          setReplayError(
            cause instanceof Error
              ? cause.message
              : "Unable to start session replay.",
          );
        }
      });
    return () => {
      cancelled = true;
      replayAbort.abort();
      stopRecording?.();
    };
  }, [
    isDemo,
    demoAuthenticated,
    demoReplayConsent,
    credentialsOpen,
    sensitivePage,
  ]);
  const visibleJourneys =
    page === "agents"
      ? journeys.filter((j) => j.resource.manifest.spec.agentic)
      : journeys;
  const search = query.toLowerCase();
  const shown = journeys.filter(
    (j) =>
      (page !== "agents" || j.resource.manifest.spec.agentic) &&
      JSON.stringify([
        j.resource.manifest.metadata,
        j.resource.manifest.spec.displayName,
        j.resource.manifest.spec.owner,
      ])
        .toLowerCase()
        .includes(search),
  );
  const chosen =
    shown.find((j) => j.resource.manifest.metadata.name === selected) ??
    (selected ? undefined : shown[0]);
  function navigate(p: Page) {
    navigateTo(workspacePath(scope.tenant, p));
    if (p === "replays") setReplayFilter("all");
    setQuery("");
  }
  function openReplays(ref: ReplayRef) {
    navigate("replays");
    setReplayFilter(ref.kind + ":" + ref.id);
  }
  function openJourney(id: string) {
    navigateTo(workspacePath(scope.tenant, "journeys", id));
  }
  function reload() {
    setRefresh((n) => n + 1);
  }
  if (isDemo && !demoAuthenticated) {
    return (
      <DemoLogin
        darkMode={darkMode}
        onToggleTheme={onToggleTheme}
        onSignIn={onDemoSignIn}
        replayConfigured={isSessionReplayConfigured()}
        replayConsent={demoReplayConsent}
      />
    );
  }
  return (
    <div className="min-h-screen bg-background text-foreground">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:fixed focus:left-3 focus:top-3 focus:z-50 focus:rounded focus:bg-white focus:p-3"
      >
        Skip to content
      </a>
      <aside className="sidebar" aria-label="Workspace navigation">
        <div className="flex h-14 shrink-0 items-center gap-2.5 border-b px-5">
          <Brand />
        </div>
        <nav className="space-y-1 px-3 py-5" aria-label="Main navigation">
          {navigation.map((n) => (
            <a
              href={workspacePath(scope.tenant, n.id)}
              key={n.id}
              className={cn(
                "nav-item",
                n.id === "connections" && "mt-6",
                activePage === n.id && "nav-active",
              )}
              onClick={(event) => {
                followLink(event, workspacePath(scope.tenant, n.id));
                if (event.defaultPrevented) {
                  setQuery("");
                  setReplayFilter("all");
                }
              }}
              aria-current={activePage === n.id ? "page" : undefined}
            >
              <n.icon className="size-4 shrink-0" aria-hidden="true" />
              <ReplayLabel>{n.label}</ReplayLabel>
            </a>
          ))}
        </nav>
        <div className="mt-auto border-t p-3">
          <Button
            variant="ghost"
            size="sm"
            className="w-full justify-start text-muted-foreground"
            onClick={onCredentials}
          >
            <Settings2 className="size-4" aria-hidden="true" />
            <ReplayLabel>API connection</ReplayLabel>
          </Button>
        </div>
      </aside>
      <div className="workspace">
        <WorkspaceHeader
          tenants={tenants}
          tenant={scope.tenant}
          role={scope.role}
          demo={data?.workspace.mode === "demo"}
          loading={loading}
          onTenant={onTenant}
          onRefresh={reload}
          onCredentials={onCredentials}
          identityLabel={
            identityLabel ?? (isDemo ? "Demo administrator" : undefined)
          }
          onSignOut={
            onSignOut || isDemo
              ? () => {
                  nativeCapture.discard();
                  (onSignOut ?? onDemoSignOut)();
                }
              : undefined
          }
          darkMode={darkMode}
          onToggleTheme={onToggleTheme}
          replayConfigured={isSessionReplayConfigured()}
          replayEnabled={demoReplayConsent}
          onToggleReplay={onToggleDemoReplay}
        />
        <nav className="mobile-navigation" aria-label="Mobile navigation">
          <Select
            value={activePage}
            onValueChange={(value) => {
              const item = navigation.find((n) => n.id === value);
              if (item) navigate(item.id);
            }}
          >
            <SelectTrigger className="w-full" aria-label="Navigate to page">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {navigation.map((n) => (
                <SelectItem key={n.id} value={n.id}>
                  <n.icon className="size-4" aria-hidden="true" />
                  <ReplayLabel>{n.label}</ReplayLabel>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </nav>
        <main
          id="main"
          ref={main}
          tabIndex={-1}
          className="workspace-main outline-none"
        >
          {nativeState.status !== "idle" ? (
            <div
              data-replay-block
              role="status"
              className="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-md border bg-muted/30 p-3 text-xs"
            >
              <span>
                {nativeState.status === "ready"
                  ? "A masked recording is ready to save."
                  : nativeState.status === "paused"
                    ? "Session recording paused on this screen."
                    : "Recording this tab. Private data is masked."}
              </span>
              <div className="flex gap-2">
                {nativeState.status !== "ready" ? (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      nativeCapture.stop();
                      navigate("replays");
                    }}
                  >
                    <ReplayLabel>Stop recording</ReplayLabel>
                  </Button>
                ) : (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => navigate("replays")}
                  >
                    Open recording
                  </Button>
                )}
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => nativeCapture.discard()}
                >
                  Discard
                </Button>
              </div>
            </div>
          ) : null}
          {data &&
          ((slo && !objectives.some((o) => o.slo.id === slo)) ||
            (selected &&
              !journeys.some(
                (j) => j.resource.manifest.metadata.name === selected,
              ))) ? (
            <div className="empty-state">
              <h1>Resource not found</h1>
              <p>This resource is unavailable in this workspace.</p>
              <Button onClick={() => navigate(page)}>
                Back to {copy[page][0]}
              </Button>
            </div>
          ) : slo && objectives.some((o) => o.slo.id === slo) ? (
            <ObjectiveDetail
              refreshKey={updated?.getTime() ?? 0}
              objective={objectives.find((o) => o.slo.id === slo)}
              scope={scope}
              journeys={journeys}
              onClose={() => setSLO("")}
              onJourney={openJourney}
              onConnections={() => navigate("connections")}
              onReplays={() => {
                const obj = objectives.find((o) => o.slo.id === slo);
                if (obj)
                  openReplays({
                    kind: "ServiceLevelObjective",
                    id: slo,
                    revision: obj.slo.revision ?? 1,
                  });
              }}
              onRefresh={reload}
            />
          ) : (
            <>
              <PageHeader
                replayTitle
                title={copy[page][0]}
                description={copy[page][1]}
              >
                {page === "objectives" ? (
                  canEdit(scope) ? (
                    <Suspense
                      fallback={<span role="status">Loading YAML editor…</span>}
                    >
                      <Button
                        variant="outline"
                        onClick={() => navigate("intents")}
                      >
                        <ReplayLabel>Create from intent</ReplayLabel>
                      </Button>
                      <ObjectiveCreate scope={scope} onCreated={reload} />
                    </Suspense>
                  ) : null
                ) : page === "journeys" || page === "agents" ? (
                  <ManifestEditor
                    scope={scope}
                    onApplied={reload}
                    onProposed={
                      data?.workspace.workflowsConfigured
                        ? () => navigate("workflows")
                        : undefined
                    }
                  />
                ) : null}
              </PageHeader>
              {error ? (
                <div
                  role="alert"
                  className="mb-6 flex flex-wrap items-center justify-between gap-3 rounded-md border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900"
                >
                  <span>{error}</span>
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => onCredentials()}
                    >
                      Set API token
                    </Button>
                    <Button size="sm" variant="outline" onClick={reload}>
                      <ReplayLabel>Retry</ReplayLabel>
                    </Button>
                  </div>
                </div>
              ) : null}
              {replayError ? (
                <div
                  role="alert"
                  className="mb-6 rounded-md border p-4 text-sm"
                >
                  Session replay could not start: {replayError}
                </div>
              ) : null}
              {loading && !data ? (
                <div
                  role="status"
                  className="empty-state rounded-md border bg-white"
                >
                  Loading workspace…
                </div>
              ) : data ? (
                <>
                  {page === "journeys" ||
                  page === "agents" ||
                  page === "objectives" ? (
                    <dl className="summary-grid">
                      {(page === "objectives"
                        ? [
                            { label: "Objectives", value: objectives.length },
                            {
                              label: "Healthy",
                              value: objectives.filter(
                                (o) => healthOf(o.status) === "healthy",
                              ).length,
                            },
                            {
                              label: "Needs attention",
                              value: objectives.filter((o) =>
                                ["degraded", "at_risk"].includes(
                                  healthOf(o.status),
                                ),
                              ).length,
                            },
                            {
                              label: "Without full coverage",
                              value: objectives.filter(
                                (o) => healthOf(o.status) === "unknown",
                              ).length,
                            },
                          ]
                        : [
                            {
                              label:
                                page === "agents"
                                  ? "Agentic journeys"
                                  : "Journeys",
                              value: visibleJourneys.length,
                            },
                            {
                              label: "Healthy",
                              value: visibleJourneys.filter(
                                (j) => j.health === "healthy",
                              ).length,
                            },
                            {
                              label: "Linked objectives",
                              value: new Set(
                                visibleJourneys.flatMap((j) =>
                                  j.links.map((l) => l.link.id),
                                ),
                              ).size,
                            },
                            {
                              label: "Needs attention",
                              value: visibleJourneys.filter(
                                (j) =>
                                  j.health !== "healthy" &&
                                  j.health !== "unknown",
                              ).length,
                            },
                          ]
                      ).map((metric) => (
                        <div className="summary-metric" key={metric.label}>
                          <dt>
                            <ReplayLabel>{metric.label}</ReplayLabel>
                          </dt>
                          <dd>{metric.value}</dd>
                        </div>
                      ))}
                    </dl>
                  ) : null}
                  {page === "journeys" ||
                  page === "agents" ||
                  page === "objectives" ? (
                    <div className="resource-toolbar">
                      <div className="relative w-full max-w-sm">
                        <Search className="pointer-events-none absolute left-3 top-2.5 size-4 text-muted-foreground" />
                        <Input
                          className="bg-white pl-9"
                          aria-label="Search journeys or objectives"
                          placeholder={
                            page === "objectives"
                              ? "Search objectives"
                              : "Search journeys"
                          }
                          value={query}
                          onChange={(e) => setQuery(e.target.value)}
                        />
                      </div>
                      <span className="hidden whitespace-nowrap text-xs text-muted-foreground sm:block">
                        {page === "objectives"
                          ? "Sorted by remaining budget"
                          : `${shown.length} journeys`}
                      </span>
                    </div>
                  ) : null}
                  {page === "investigations" ? (
                    <Suspense fallback={<p role="status">Loading reviews…</p>}>
                      <Investigations
                        key={scope.tenant}
                        scope={scope}
                        objectives={objectives}
                        journeys={journeys}
                        onSLO={(id) => {
                          setSLO(id);
                        }}
                        onJourney={openJourney}
                      />
                    </Suspense>
                  ) : page === "audit" ? (
                    <Suspense
                      fallback={<p role="status">Loading audit log…</p>}
                    >
                      <AuditLog key={scope.tenant} scope={scope} />
                    </Suspense>
                  ) : page === "replays" ? (
                    <Suspense
                      fallback={<p role="status">Loading session replay…</p>}
                    >
                      <SessionReplays
                        key={scope.tenant}
                        scope={scope}
                        capture={nativeCapture}
                        initialFilter={replayFilter}
                        options={replayOptions}
                        onResource={(ref) => {
                          if (ref.kind === "ServiceLevelObjective") {
                            setSLO(ref.id);
                          } else if (ref.kind === "CriticalUserJourney") {
                            openJourney(ref.id);
                          } else navigate("alerts");
                        }}
                      />
                    </Suspense>
                  ) : page === "home" ? (
                    <Suspense
                      fallback={
                        <p role="status" className="empty-state">
                          Loading overview…
                        </p>
                      }
                    >
                      <Home
                        data={data}
                        onNavigate={navigate}
                        onSLO={setSLO}
                        onJourney={openJourney}
                      />
                    </Suspense>
                  ) : page === "maps" ||
                    page === "bindings" ||
                    page === "intents" ? (
                    <Suspense
                      fallback={
                        <p role="status" className="empty-state">
                          Loading telemetry workspace…
                        </p>
                      }
                    >
                      {page === "intents" ? (
                        <IntentAuthoring
                          key={scope.tenant}
                          scope={scope}
                          journeys={journeys}
                          onApplied={reload}
                        />
                      ) : page === "maps" ? (
                        <DependencyMaps
                          scope={scope}
                          onSLO={setSLO}
                          onJourney={openJourney}
                        />
                      ) : (
                        <MeasurementBindings scope={scope} />
                      )}
                    </Suspense>
                  ) : page === "connections" ||
                    page === "access" ||
                    page === "clients" ? (
                    <Suspense
                      fallback={
                        <p role="status" className="empty-state">
                          Loading settings…
                        </p>
                      }
                    >
                      {page === "connections" ? (
                        <Connections scope={scope} />
                      ) : page === "access" ? (
                        <AccessControl scope={scope} />
                      ) : (
                        <Clients scope={scope} />
                      )}
                    </Suspense>
                  ) : page === "journeys" || page === "agents" ? (
                    shown.length ? (
                      <div className="resource-split">
                        <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-1">
                          {shown.map((j) => (
                            <button
                              key={j.resource.manifest.metadata.name}
                              aria-pressed={chosen === j}
                              onClick={() =>
                                setSelected(j.resource.manifest.metadata.name)
                              }
                              className={cn(
                                "journey-option",
                                chosen === j && "journey-selected",
                              )}
                            >
                              <div className="mb-3 flex items-center justify-between">
                                <div
                                  className={cn(
                                    "rounded border p-1.5",
                                    chosen === j
                                      ? "border-teal-200 bg-teal-50"
                                      : "border-slate-200 bg-slate-50",
                                  )}
                                >
                                  {j.resource.manifest.spec.agentic ? (
                                    <Bot className="size-4 text-slate-500" />
                                  ) : (
                                    <Waypoints className="size-4 text-primary" />
                                  )}
                                </div>
                                <Status value={j.health} />
                              </div>
                              <h2 className="text-sm font-semibold">
                                {j.resource.manifest.spec.displayName}
                              </h2>
                              <p className="mt-1 text-xs text-muted-foreground">
                                {j.resource.manifest.spec.owner}
                              </p>
                              <div className="mt-3 flex gap-3 text-xs text-muted-foreground">
                                <span>
                                  {j.resource.manifest.spec.steps.length} steps
                                </span>
                                <span>{j.links.length} SLOs</span>
                                <span className="ml-auto">
                                  v{j.resource.revision}
                                </span>
                              </div>
                            </button>
                          ))}
                        </div>
                        {chosen ? (
                          <div className="min-w-0">
                            <Suspense
                              fallback={
                                <p role="status" className="empty-state">
                                  Loading journey…
                                </p>
                              }
                            >
                              <JourneyDetail
                                key={chosen.resource.manifest.metadata.name}
                                refreshKey={updated?.getTime() ?? 0}
                                journey={chosen}
                                scope={scope}
                                onSLO={setSLO}
                                onReplays={() =>
                                  openReplays({
                                    kind: "CriticalUserJourney",
                                    id: chosen.resource.manifest.metadata.name,
                                    revision: chosen.resource.revision,
                                  })
                                }
                                onRefresh={reload}
                                onProposed={
                                  data?.workspace.workflowsConfigured
                                    ? () => navigate("workflows")
                                    : undefined
                                }
                                evidenceOnly={page === "agents"}
                              />
                            </Suspense>
                          </div>
                        ) : null}
                      </div>
                    ) : (
                      <div className="empty-state rounded-md border bg-white">
                        <Waypoints className="mx-auto mb-4 size-8 text-slate-300" />
                        <h2 className="text-base font-semibold text-slate-700">
                          {query
                            ? "No journeys match your search"
                            : page === "agents"
                              ? "No agentic contracts yet"
                              : "No critical user journeys"}
                        </h2>
                        <p className="mx-auto mt-2 max-w-md text-sm">
                          {query
                            ? "Try a journey name, team, or project."
                            : "Create a journey definition and link its service level objectives."}
                        </p>
                      </div>
                    )
                  ) : page === "objectives" ? (
                    <Objectives
                      items={objectives.filter((o) =>
                        JSON.stringify(o.slo).toLowerCase().includes(search),
                      )}
                      journeys={journeys}
                      onSelect={setSLO}
                      onJourney={openJourney}
                    />
                  ) : page === "workflows" ? (
                    data.workspace.workflowsConfigured ? (
                      <Suspense
                        fallback={
                          <p role="status" className="empty-state">
                            Loading workflow tools…
                          </p>
                        }
                      >
                        <Workflows scope={scope} journeys={journeys} />
                      </Suspense>
                    ) : (
                      <p className="empty-state rounded-md border bg-white">
                        Workflows are not configured for this tenant.
                      </p>
                    )
                  ) : (
                    <Card className="gap-0 py-0">
                      <CardContent className="p-0">
                        <Table>
                          <TableHeader>
                            <TableRow>
                              <TableHead className="pl-6">Transition</TableHead>
                              <TableHead>Objective</TableHead>
                              <TableHead>Burn (short / long)</TableHead>
                              <TableHead>Delivery</TableHead>
                              <TableHead>Time</TableHead>
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {events.map((d) => (
                              <TableRow key={d.event.id}>
                                <TableCell className="py-5 pl-6">
                                  <Status
                                    value={
                                      d.event.kind === "firing"
                                        ? "degraded"
                                        : "healthy"
                                    }
                                  />
                                </TableCell>
                                <TableCell>
                                  <button
                                    className="font-medium hover:text-primary"
                                    onClick={() => setSLO(d.event.sloId)}
                                  >
                                    {d.event.sloId}
                                  </button>
                                </TableCell>
                                <TableCell className="font-mono text-xs">
                                  {d.event.shortBurn.toFixed(2)}× /{" "}
                                  {d.event.longBurn.toFixed(2)}×
                                </TableCell>
                                <TableCell className="text-xs">
                                  {d.status} · {d.attempts} attempts
                                  {d.lastError ? (
                                    <div className="text-destructive">
                                      {d.lastError}
                                    </div>
                                  ) : null}
                                </TableCell>
                                <TableCell className="text-xs text-muted-foreground">
                                  {date(d.event.time)}
                                </TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                        {!events.length ? (
                          <p className="empty-state">
                            No alert transitions have been recorded.
                          </p>
                        ) : null}
                      </CardContent>
                    </Card>
                  )}
                </>
              ) : null}
            </>
          )}
        </main>
        <WorkspaceFooter
          source={
            !data
              ? "Unavailable"
              : data.workspace.source.configured
                ? (data.workspace.source.connectionCount ?? 1) > 1
                  ? `${data.workspace.source.connectionCount} metrics connections`
                  : data.workspace.source.kind === "mimir"
                    ? "Mimir"
                    : "Prometheus"
                : "Manual ingestion"
          }
          updated={updated}
          loading={loading}
          error={!!error}
        />
      </div>
    </div>
  );
}
