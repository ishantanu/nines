import { ReplayLabel } from "@/components/replay-label";
import { Brand } from "@/components/brand";
import { type ReactNode } from "react";
import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
import { DropdownMenu } from "@astryxdesign/core/DropdownMenu";
import {
  Moon,
  RefreshCw,
  Settings2,
  Sun,
  UserRound,
  ChevronDown,
  LogOut,
  ShieldCheck,
} from "lucide-react";
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
}: {
  onPlatformAdmin?: () => void;
  identityLabel?: string;
  onSignOut?: () => void;
  darkMode: boolean;
  onToggleTheme: () => void;
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
      <div className="flex shrink-0 items-center gap-1 sm:gap-2">
        <Badge variant="outline" className="hidden capitalize sm:inline-flex">
          {role}
        </Badge>
        {demo ? (
          <Badge variant="secondary">
            <ReplayLabel>Demo</ReplayLabel>
          </Badge>
        ) : null}
        <div className="flex items-center border-l pl-2">
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
          <DropdownMenu
            alignment="end"
            button={{
              label: "Account menu",
              variant: "ghost",
              size: "sm",
              icon: <UserRound aria-hidden="true" />,
              endContent: <ChevronDown aria-hidden="true" />,
              children: (
                <span className="hidden max-w-36 truncate md:inline">
                  {identityLabel || "Account"}
                </span>
              ),
            }}
            header={
              <div className="border-b px-3 py-2.5">
                <span className="block break-words text-sm font-medium">
                  {identityLabel || "API credential"}
                </span>
                <span className="mt-1 block text-xs capitalize text-muted-foreground">
                  {role} · current tenant
                </span>
              </div>
            }
            items={[
              {
                label: "API connection",
                icon: <Settings2 aria-hidden="true" />,
                onClick: onCredentials,
              },
              ...(onPlatformAdmin
                ? [
                    {
                      label: "Platform administration",
                      icon: <ShieldCheck aria-hidden="true" />,
                      onClick: onPlatformAdmin,
                    },
                  ]
                : []),
              ...(onSignOut
                ? [
                    {
                      label: <ReplayLabel>Sign out</ReplayLabel>,
                      icon: <LogOut aria-hidden="true" />,
                      onClick: onSignOut,
                    },
                  ]
                : []),
            ]}
          />
        </div>
      </div>
    </header>
  );
}
export function PageHeader({
  replayTitle = false,
  title,
  description,
  badge,
  children,
}: {
  replayTitle?: boolean;
  title: string;
  description: string;
  badge?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <div className="page-header">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <Heading
            level={1}
            type="display-3"
            className="page-title break-words"
          >
            {replayTitle ? <ReplayLabel>{title}</ReplayLabel> : title}
          </Heading>
          {badge}
        </div>
        <Text as="p" color="secondary" className="mt-1.5 max-w-3xl break-words">
          {description}
        </Text>
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
