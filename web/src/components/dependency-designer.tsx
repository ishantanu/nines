import { useId, useRef, useState } from "react";
import { GripVertical, ArrowRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { layoutGraph, type GraphPosition } from "@/lib/topology";
import {
  connectDependency,
  moveService,
  renameService,
  type DependencyManifest,
} from "@/lib/dependency-editor";

interface Drag {
  kind: "move" | "connect";
  name: string;
  pointer: number;
  startX: number;
  startY: number;
  dx: number;
  dy: number;
}
const selectClass =
  "h-9 w-full min-w-0 rounded-md border border-input bg-background px-2 text-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring";
export function DependencyDesigner({
  value,
  disabled,
  onChange,
}: {
  value: DependencyManifest;
  disabled: boolean;
  onChange: (next: DependencyManifest) => void;
}) {
  const id = useId();
  const suppressConnectionClick = useRef(false);
  const canvas = useRef<HTMLDivElement>(null);
  const [selected, setSelected] = useState("");
  const [caller, setCaller] = useState("");
  const [callee, setCallee] = useState("");
  const [drag, setDrag] = useState<Drag | null>(null);
  const [error, setError] = useState("");
  const [status, setStatus] = useState("");
  const node =
    value.spec.services.find((n) => n.name === selected) ??
    value.spec.services[0];
  const graph = layoutGraph(value.spec.services, value.spec.dependencies);
  const positions = new Map(graph.positions);
  if (drag?.kind === "move") {
    const p = positions.get(drag.name)!;
    positions.set(drag.name, {
      x: Math.max(0, Math.min(10000, p.x + drag.dx)),
      y: Math.max(0, Math.min(10000, p.y + drag.dy)),
    });
  }
  function change(next: () => DependencyManifest, message: string) {
    if (disabled) return false;
    try {
      onChange(next());
      setError("");
      setStatus(message);
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unable to update this draft.");
      return false;
    }
  }
  function connect(from: string, to: string) {
    change(
      () => connectDependency(value, from, to),
      `Added dependency from ${from} to ${to}. Review and plan before saving.`,
    );
  }
  function start(
    event: React.PointerEvent<HTMLButtonElement>,
    kind: Drag["kind"],
    name: string,
  ) {
    if (disabled || event.button !== 0) return;
    event.preventDefault();
    suppressConnectionClick.current = false;
    event.currentTarget.focus();
    event.currentTarget.setPointerCapture(event.pointerId);
    setSelected(name);
    setDrag({
      kind,
      name,
      pointer: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      dx: 0,
      dy: 0,
    });
  }
  function pointerMove(event: React.PointerEvent<HTMLButtonElement>) {
    if (drag?.pointer === event.pointerId)
      setDrag({
        ...drag,
        dx: event.clientX - drag.startX,
        dy: event.clientY - drag.startY,
      });
  }
  function end(event: React.PointerEvent<HTMLButtonElement>) {
    if (!drag || drag.pointer !== event.pointerId) return;
    const distance = Math.hypot(
      event.clientX - drag.startX,
      event.clientY - drag.startY,
    );
    if (drag.kind === "move" && distance > 4) {
      const p = graph.positions.get(drag.name)!;
      change(
        () =>
          moveService(
            value,
            drag.name,
            {
              x: p.x + event.clientX - drag.startX,
              y: p.y + event.clientY - drag.startY,
            },
            graph.positions,
          ),
        `Moved ${drag.name}. Position is saved with the reviewed map.`,
      );
    } else if (drag.kind === "connect") {
      suppressConnectionClick.current = distance > 4;
      setCaller(drag.name);
      const target = document
        .elementFromPoint(event.clientX, event.clientY)
        ?.closest<HTMLElement>("[data-service-node]");
      const to = target?.dataset.serviceNode;
      if (distance > 4 && to && canvas.current?.contains(target))
        connect(drag.name, to);
      else
        setStatus(
          `Caller set to ${drag.name}. Choose a dependency in the form below.`,
        );
    }
    setDrag(null);
  }
  function keyboard(
    event: React.KeyboardEvent<HTMLButtonElement>,
    name: string,
  ) {
    const direction: Record<string, GraphPosition> = {
      ArrowLeft: { x: -24, y: 0 },
      ArrowRight: { x: 24, y: 0 },
      ArrowUp: { x: 0, y: -24 },
      ArrowDown: { x: 0, y: 24 },
    };
    const offset = direction[event.key];
    if (!offset || disabled) return;
    event.preventDefault();
    const p = graph.positions.get(name)!;
    change(
      () =>
        moveService(
          value,
          name,
          { x: p.x + offset.x, y: p.y + offset.y },
          graph.positions,
        ),
      `Moved ${name}.`,
    );
  }
  return (
    <div className="min-w-0 space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p
          id={`${id}-help`}
          className="max-w-2xl text-sm text-muted-foreground"
        >
          Drag a service by its name, or focus its move handle and use arrow
          keys. Draw from its connection handle to another service, or use the
          dependency form.
        </p>
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            disabled={disabled}
            onClick={() =>
              change(
                () => ({
                  ...value,
                  spec: {
                    ...value.spec,
                    services: value.spec.services.map(
                      ({ position: _position, ...n }) => n,
                    ),
                  },
                }),
                "Reset draft layout.",
              )
            }
          >
            Reset layout
          </Button>
          <Button
            size="sm"
            disabled={disabled || value.spec.services.length >= 40}
            onClick={() => {
              let number = value.spec.services.length + 1;
              while (
                value.spec.services.some((n) => n.name === `service-${number}`)
              )
                number++;
              const name = `service-${number}`;
              change(
                () => ({
                  ...value,
                  spec: {
                    ...value.spec,
                    services: [
                      ...value.spec.services,
                      { name, displayName: name, sloRefs: [] },
                    ],
                  },
                }),
                `Added ${name}.`,
              );
              setSelected(name);
            }}
          >
            Add service
          </Button>
        </div>
      </div>
      <div
        ref={canvas}
        role="region"
        aria-label="Dependency map editor"
        tabIndex={0}
        className="h-96 max-w-full overflow-auto rounded-md border bg-muted/30 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
      >
        <div
          className="relative"
          style={{
            width: Math.max(
              graph.width,
              ...Array.from(positions.values(), (p) => p.x + 228),
            ),
            height: Math.max(
              graph.height,
              ...Array.from(positions.values(), (p) => p.y + 116),
            ),
          }}
        >
          <svg
            className="pointer-events-none absolute inset-0 size-full"
            aria-hidden="true"
          >
            <defs>
              <marker
                id={`${id}-arrow`}
                markerWidth="8"
                markerHeight="8"
                refX="7"
                refY="4"
                orient="auto"
              >
                <path d="M0,0 L8,4 L0,8" fill="var(--muted-foreground)" />
              </marker>
            </defs>
            {value.spec.dependencies.map((e) => {
              const from = positions.get(e.caller),
                to = positions.get(e.callee);
              if (!from || !to) return null;
              return (
                <path
                  key={`${e.caller}/${e.callee}`}
                  d={`M${from.x + 192},${from.y + 40} C${from.x + 236},${from.y + 40} ${to.x - 44},${to.y + 40} ${to.x},${to.y + 40}`}
                  fill="none"
                  stroke="var(--muted-foreground)"
                  strokeWidth={2}
                  markerEnd={`url(#${id}-arrow)`}
                />
              );
            })}
            {drag?.kind === "connect" ? (
              <line
                x1={positions.get(drag.name)!.x + 192}
                y1={positions.get(drag.name)!.y + 40}
                x2={positions.get(drag.name)!.x + 192 + drag.dx}
                y2={positions.get(drag.name)!.y + 40 + drag.dy}
                stroke="var(--primary)"
                strokeWidth={2}
                strokeDasharray="5 4"
              />
            ) : null}
          </svg>
          {value.spec.services.map((n) => {
            const p = positions.get(n.name)!;
            return (
              <div
                key={n.name}
                data-service-node={n.name}
                className={`absolute h-20 w-48 rounded-md border bg-card shadow-sm ${node?.name === n.name ? "border-primary ring-1 ring-primary" : "border-input"}`}
                style={{ left: p.x, top: p.y }}
              >
                <button
                  type="button"
                  disabled={disabled}
                  aria-label={`Move service ${n.name}`}
                  aria-describedby={`${id}-help`}
                  className="flex h-11 w-full touch-none items-center gap-1 rounded-t-md px-2 text-left text-sm font-medium focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                  onClick={() => setSelected(n.name)}
                  onPointerDown={(e) => start(e, "move", n.name)}
                  onPointerMove={pointerMove}
                  onPointerUp={end}
                  onPointerCancel={() => setDrag(null)}
                  onLostPointerCapture={() => setDrag(null)}
                  onKeyDown={(e) => keyboard(e, n.name)}
                >
                  <GripVertical
                    className="size-4 shrink-0 text-muted-foreground"
                    aria-hidden="true"
                  />
                  <span className="truncate">{n.displayName}</span>
                </button>
                <p className="px-3 text-sm text-muted-foreground">
                  {n.sloRefs.length} linked SLOs
                </p>
                <button
                  type="button"
                  disabled={disabled}
                  aria-label={`Draw dependency from ${n.name}`}
                  className="absolute -right-4 top-6 flex size-8 touch-none items-center justify-center rounded-full border border-input bg-card text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                  onPointerDown={(e) => start(e, "connect", n.name)}
                  onPointerMove={pointerMove}
                  onPointerUp={end}
                  onPointerCancel={() => setDrag(null)}
                  onLostPointerCapture={() => setDrag(null)}
                  onClick={() => {
                    if (suppressConnectionClick.current) {
                      suppressConnectionClick.current = false;
                      return;
                    }
                    setCaller(n.name);
                    setStatus(
                      `Caller set to ${n.name}. Choose a dependency below.`,
                    );
                  }}
                >
                  <ArrowRight className="size-4" aria-hidden="true" />
                </button>
              </div>
            );
          })}
          {!value.spec.services.length ? (
            <p className="p-6 text-sm text-muted-foreground">
              Add a service or discover resources from Kubernetes to start this
              map.
            </p>
          ) : null}
        </div>
      </div>
      {error ? (
        <p role="alert" className="text-sm text-[var(--status-degraded)]">
          {error}
        </p>
      ) : null}
      <p role="status" className="min-h-5 text-sm text-muted-foreground">
        {status}
      </p>
      {node ? (
        <fieldset
          disabled={disabled}
          className="grid gap-4 rounded-md border p-4 sm:grid-cols-2"
        >
          <legend className="px-1 text-sm font-semibold">
            Selected service
          </legend>
          <div className="space-y-2">
            <Label htmlFor={`${id}-service`}>Service identity</Label>
            <Input
              key={`name-${node.name}`}
              id={`${id}-service`}
              defaultValue={node.name}
              maxLength={200}
              onBlur={(e) => {
                if (e.target.value !== node.name) {
                  const next = e.target.value;
                  if (
                    change(
                      () => renameService(value, node.name, next),
                      "Updated service identity. Review linked SLO ownership before saving.",
                    )
                  )
                    setSelected(next);
                }
              }}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor={`${id}-display`}>Display name</Label>
            <Input
              id={`${id}-display`}
              value={node.displayName}
              maxLength={200}
              onChange={(e) =>
                onChange({
                  ...value,
                  spec: {
                    ...value.spec,
                    services: value.spec.services.map((n) =>
                      n.name === node.name
                        ? { ...n, displayName: e.target.value }
                        : n,
                    ),
                  },
                })
              }
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor={`${id}-slos`}>Linked SLO names</Label>
            <Input
              key={`refs-${node.name}`}
              id={`${id}-slos`}
              defaultValue={node.sloRefs.join(", ")}
              onBlur={(e) =>
                change(
                  () => ({
                    ...value,
                    spec: {
                      ...value.spec,
                      services: value.spec.services.map((n) =>
                        n.name === node.name
                          ? {
                              ...n,
                              sloRefs: e.target.value
                                .split(",")
                                .map((v) => v.trim())
                                .filter(Boolean),
                            }
                          : n,
                      ),
                    },
                  }),
                  "Updated SLO links.",
                )
              }
            />
            <p className="text-sm text-muted-foreground">
              Comma-separated names. Each SLO must belong to this service.
            </p>
          </div>
          <div className="flex items-end">
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                change(
                  () => ({
                    ...value,
                    spec: {
                      ...value.spec,
                      services: value.spec.services.filter(
                        (n) => n.name !== node.name,
                      ),
                      dependencies: value.spec.dependencies.filter(
                        (e) => e.caller !== node.name && e.callee !== node.name,
                      ),
                    },
                  }),
                  `Removed ${node.name} from this draft.`,
                );
                setSelected("");
              }}
            >
              Remove service
            </Button>
          </div>
        </fieldset>
      ) : null}
      <form
        className="grid items-end gap-3 rounded-md border p-4 sm:grid-cols-[1fr_1fr_auto]"
        onSubmit={(e) => {
          e.preventDefault();
          connect(caller, callee);
        }}
      >
        <div className="space-y-2">
          <Label htmlFor={`${id}-caller`}>Caller</Label>
          <select
            id={`${id}-caller`}
            value={caller}
            disabled={disabled}
            onChange={(e) => setCaller(e.target.value)}
            className={selectClass}
          >
            <option value="">Choose a service</option>
            {value.spec.services.map((n) => (
              <option key={n.name} value={n.name}>
                {n.displayName} ({n.name})
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-2">
          <Label htmlFor={`${id}-callee`}>Depends on</Label>
          <select
            id={`${id}-callee`}
            value={callee}
            disabled={disabled}
            onChange={(e) => setCallee(e.target.value)}
            className={selectClass}
          >
            <option value="">Choose a service</option>
            {value.spec.services
              .filter((n) => n.name !== caller)
              .map((n) => (
                <option key={n.name} value={n.name}>
                  {n.displayName} ({n.name})
                </option>
              ))}
          </select>
        </div>
        <Button
          type="submit"
          variant="outline"
          disabled={disabled || !caller || !callee}
        >
          Add dependency
        </Button>
      </form>
      <ul className="space-y-2" aria-label="Draft dependencies">
        {value.spec.dependencies.map((edge, index) => (
          <li
            key={`${edge.caller}/${edge.callee}`}
            className="flex flex-wrap items-center gap-3 rounded-md border p-3 text-sm"
          >
            <span className="min-w-0 flex-1 break-words">
              {edge.caller} → {edge.callee}
              {edge.description ? (
                <span className="mt-1 block text-muted-foreground">
                  {edge.description}
                </span>
              ) : null}
            </span>
            <label className="flex min-h-8 items-center gap-2">
              <input
                type="checkbox"
                checked={edge.critical}
                disabled={disabled}
                onChange={(e) =>
                  onChange({
                    ...value,
                    spec: {
                      ...value.spec,
                      dependencies: value.spec.dependencies.map((old, i) =>
                        i === index
                          ? { ...old, critical: e.target.checked }
                          : old,
                      ),
                    },
                  })
                }
              />
              Critical
            </label>
            <Button
              size="sm"
              variant="outline"
              disabled={disabled}
              aria-label={`Remove dependency ${edge.caller} to ${edge.callee}`}
              onClick={() =>
                onChange({
                  ...value,
                  spec: {
                    ...value.spec,
                    dependencies: value.spec.dependencies.filter(
                      (_, i) => i !== index,
                    ),
                  },
                })
              }
            >
              Remove
            </Button>
          </li>
        ))}
      </ul>
    </div>
  );
}
