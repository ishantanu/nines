"use client";

import * as React from "react";
import { Tab, TabList } from "@astryxdesign/core/TabList";
import { Tooltip } from "@astryxdesign/core/Tooltip";
import { cn } from "cn";
import { replayLabels } from "@/lib/replay-labels";

// Adapter over Astryx TabList/Tab. Astryx owns the tab strip only, so this
// supplies the selected-value state and the tab panels around it.
type TabsState = {
  value: string;
  select: (value: string) => void;
  base: string;
  disabled: React.RefObject<Set<string>>;
};
const TabsContext = React.createContext<TabsState | null>(null);
function useTabs(): TabsState {
  const state = React.useContext(TabsContext);
  if (!state) throw new Error("Tabs components must be rendered inside Tabs");
  return state;
}
const panelId = (base: string, value: string) => `${base}-panel-${value}`;

function Tabs({
  value: controlled,
  defaultValue = "",
  onValueChange,
  className,
  children,
  ...props
}: Omit<React.ComponentProps<"div">, "dir" | "defaultValue" | "onChange"> & {
  value?: string;
  defaultValue?: string;
  onValueChange?: (value: string) => void;
  orientation?: "horizontal" | "vertical";
}) {
  const [uncontrolled, setUncontrolled] = React.useState(defaultValue);
  const base = React.useId();
  const disabled = React.useRef(new Set<string>());
  const value = controlled ?? uncontrolled;
  const select = React.useCallback(
    (next: string) => {
      if (disabled.current.has(next)) return;
      if (controlled === undefined) setUncontrolled(next);
      onValueChange?.(next);
    },
    [controlled, onValueChange],
  );
  const { orientation: _orientation, ...rest } = props;
  return (
    <TabsContext.Provider value={{ value, select, base, disabled }}>
      <div
        data-slot="tabs"
        className={cn("flex min-w-0 flex-col gap-2", className)}
        {...rest}
      >
        {children}
      </div>
    </TabsContext.Provider>
  );
}

function TabsList({
  className,
  children,
}: {
  className?: string;
  children: React.ReactNode;
  variant?: "default" | "line";
}) {
  const { value, select } = useTabs();
  return (
    <TabList
      role="tablist"
      data-slot="tabs-list"
      className={className}
      value={value}
      onChange={select}
      size="sm"
    >
      {children}
    </TabList>
  );
}

function textOf(node: React.ReactNode): string {
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(textOf).join("");
  if (React.isValidElement<{ children?: React.ReactNode }>(node))
    return textOf(node.props.children);
  return "";
}

function TabsTrigger({
  value,
  disabled = false,
  title,
  className,
  children,
}: {
  value: string;
  disabled?: boolean;
  title?: string;
  className?: string;
  children: React.ReactNode;
}) {
  const { base, disabled: blocked } = useTabs();
  React.useLayoutEffect(() => {
    if (disabled) blocked.current.add(value);
    return () => {
      blocked.current.delete(value);
    };
  }, [blocked, disabled, value]);
  const label = textOf(children);
  const tab = (
    <Tab
      data-slot="tabs-trigger"
      className={className}
      value={value}
      label={label}
      panelId={panelId(base, value)}
      aria-disabled={disabled || undefined}
      data-replay-label={replayLabels.has(label) ? label : undefined}
    />
  );
  // Explains why a tab is unavailable; Tab itself has no tooltip prop.
  return title ? <Tooltip content={title}>{tab}</Tooltip> : tab;
}

function TabsContent({
  value,
  forceMount,
  className,
  children,
  ...props
}: Omit<React.ComponentProps<"div">, "dir"> & {
  value: string;
  forceMount?: true;
}) {
  const { value: active, base } = useTabs();
  const selected = active === value;
  if (!selected && !forceMount) return null;
  return (
    <div
      {...props}
      id={panelId(base, value)}
      role="tabpanel"
      data-slot="tabs-content"
      data-state={selected ? "active" : "inactive"}
      hidden={!selected}
      className={cn("min-w-0 flex-1 outline-none", className)}
    >
      {children}
    </div>
  );
}

export { Tabs, TabsList, TabsTrigger, TabsContent };
