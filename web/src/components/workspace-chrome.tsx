import { ReplayLabel } from "@/components/replay-label";
import { Brand } from "@/components/brand";
import type { ReactNode } from "react";
import { Moon, RefreshCw, Settings2, Sun, Video, VideoOff } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { TenantSelector } from "@/components/tenant-selector";
import type { Tenant } from "@/lib/api";
import { cn } from "@/lib/utils";

export function WorkspaceHeader({
  tenants,
  tenant,
  role,
  demo,
  loading,
  onTenant,
  onRefresh,
  onCredentials,
  identityLabel,
  onSignOut,
  onPlatformAdmin,
  darkMode,
  onToggleTheme,
  replayConfigured,
  replayEnabled,
  onToggleReplay,
}: {
  onPlatformAdmin?: () => void;
  identityLabel?: string;
  onSignOut?: () => void;
  darkMode: boolean;
  onToggleTheme: () => void;
  replayConfigured: boolean;
  replayEnabled: boolean;
  onToggleReplay: () => void;
  tenants: Tenant[];
  tenant: string;
  role: string;
  demo: boolean;
  loading: boolean;
  onTenant: (id: string) => void;
  onRefresh: () => void;
  onCredentials: () => void;
}) {
  return (
    <header className="app-header">
      <div className="flex min-w-0 items-center gap-2 text-sm">
        <span className="flex items-center gap-2 font-semibold lg:hidden">
          <Brand className="h-6" />
        </span>
        <TenantSelector
          tenants={tenants}
          selected={tenant}
          onChange={onTenant}
        />
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {onPlatformAdmin ? (
          <Button variant="outline" size="sm" onClick={onPlatformAdmin}>
            Administration
          </Button>
        ) : null}
        {identityLabel ? (
          <span
            className="max-w-40 truncate text-xs text-muted-foreground"
            title={identityLabel}
          >
            {identityLabel}
          </span>
        ) : null}
        {onSignOut ? (
          <Button variant="ghost" size="sm" onClick={onSignOut}>
            <ReplayLabel>Sign out</ReplayLabel>
          </Button>
        ) : null}
        <Badge variant="outline" className="capitalize">
          {role}
        </Badge>
        {demo ? (
          <Badge variant="secondary">
            <ReplayLabel>Demo</ReplayLabel>
          </Badge>
        ) : null}
        <div className="flex items-center border-l pl-2">
          {demo && replayConfigured ? (
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={onToggleReplay}
              aria-label={
                replayEnabled
                  ? "Stop operator diagnostics recording"
                  : "Allow operator diagnostics recording"
              }
              aria-pressed={replayEnabled}
              title={
                replayEnabled
                  ? "Stop operator diagnostics recording"
                  : "Allow operator diagnostics recording"
              }
            >
              {replayEnabled ? (
                <Video className="size-4" aria-hidden="true" />
              ) : (
                <VideoOff className="size-4" aria-hidden="true" />
              )}
            </Button>
          ) : null}
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onToggleTheme}
            aria-label={
              darkMode ? "Switch to light mode" : "Switch to dark mode"
            }
            aria-pressed={darkMode}
            title={darkMode ? "Switch to light mode" : "Switch to dark mode"}
          >
            {darkMode ? (
              <Sun className="size-4" aria-hidden="true" />
            ) : (
              <Moon className="size-4" aria-hidden="true" />
            )}
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onRefresh}
            aria-label="Refresh workspace"
          >
            <RefreshCw
              className={cn("size-4", loading && "animate-spin")}
              aria-hidden="true"
            />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onCredentials}
            aria-label="API connection"
            className="lg:hidden"
          >
            <Settings2 className="size-4" aria-hidden="true" />
          </Button>
        </div>
      </div>
    </header>
  );
}
export function PageHeader({
  replayTitle = false,
  title,
  description,
  children,
}: {
  replayTitle?: boolean;
  title: string;
  description: string;
  children?: ReactNode;
}) {
  return (
    <div className="page-header">
      <div className="min-w-0">
        <h1 className="break-words text-2xl font-semibold tracking-tight">
          {replayTitle ? <ReplayLabel>{title}</ReplayLabel> : title}
        </h1>
        <p className="mt-1.5 max-w-3xl break-words text-sm leading-6 text-muted-foreground">
          {description}
        </p>
      </div>
      {children ? (
        <div className="flex max-w-full flex-wrap items-center gap-2">
          {children}
        </div>
      ) : null}
    </div>
  );
}
export function WorkspaceFooter({
  source,
  updated,
  loading,
  error,
}: {
  source: string;
  updated: Date | null;
  loading: boolean;
  error: boolean;
}) {
  return (
    <footer className="workspace-footer" aria-label="Workspace status">
      <span>
        Telemetry connection:{" "}
        <span className="font-medium text-foreground">{source}</span>
      </span>
      <span className={error ? "status-at-risk" : undefined}>
        {error
          ? "Refresh failed"
          : loading
            ? "Refreshing data…"
            : updated
              ? `Last updated ${updated.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })}`
              : "Awaiting data"}
      </span>
    </footer>
  );
}
