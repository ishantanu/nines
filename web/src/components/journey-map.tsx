import { useCallback, useEffect, useRef, useState } from "react";
import {
  Bot,
  UserRound,
  Server,
  ShieldCheck,
  ZoomIn,
  ZoomOut,
  Maximize2,
  X,
} from "lucide-react";
import type { Journey, Step, Health } from "@/lib/api";
import { stepLayers } from "@/lib/api";
import { useChartSize } from "@/lib/use-chart-size";
import { SLIValue } from "./sli-value";
import { Status } from "./status";
import { Button } from "./ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogTrigger,
} from "./ui/dialog";

const actors = {
  agent: Bot,
  user: UserRound,
  service: Server,
  human: ShieldCheck,
};
function stepHealth(step: Step, journey: Journey): Health {
  const values = step.sloRefs.map(
    (id) =>
      journey.links.find((link) => link.link.id === id)?.health ?? "unknown",
  );
  if (!values.length) return "unknown";
  return values.includes("degraded")
    ? "degraded"
    : values.includes("at_risk")
      ? "at_risk"
      : values.includes("unknown")
        ? "unknown"
        : "healthy";
}
export function JourneyMap({
  journey,
  onSLO,
  expanded = false,
}: {
  journey: Journey;
  onSLO: (id: string) => void;
  expanded?: boolean;
}) {
  const steps = journey.resource.manifest.spec.steps;
  const layers = stepLayers(steps);
  const stepButtons = useRef(new Map<string, HTMLButtonElement>());
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const selected = steps.find((step) => step.id === selectedID);
  const [view, setView] = useState<"map" | "list">("map");
  const [fit, setFit] = useState(true);
  const [manualZoom, setManualZoom] = useState(1);
  const { ref: measure, width, height } = useChartSize();
  const viewport = useRef<HTMLDivElement | null>(null);
  const viewportRef = useCallback(
    (element: HTMLDivElement | null) => {
      viewport.current = element;
      measure(element);
    },
    [measure],
  );
  const drag = useRef<{
    x: number;
    y: number;
    left: number;
    top: number;
  } | null>(null);
  const contentWidth = Math.max(320, layers.length * 304 - 48);
  const contentHeight = Math.max(
    200,
    Math.max(0, ...layers.map((layer) => layer.length)) * 132 + 64,
  );
  const fitZoom = Math.max(
    0.25,
    Math.min(1, (width - 32) / contentWidth, (height - 32) / contentHeight),
  );
  const zoom = fit ? fitZoom : manualZoom;
  const positions = new Map(
    layers.flatMap((layer, column) =>
      layer.map(
        (step, row) =>
          [step.id, { x: 24 + column * 304, y: 52 + row * 132 }] as const,
      ),
    ),
  );
  function changeZoom(next: number) {
    setManualZoom(Math.max(0.25, Math.min(1.5, next)));
    setFit(false);
  }
  function fitJourney() {
    setFit(true);
    viewport.current?.scrollTo?.({ left: 0, top: 0 });
  }
  useEffect(() => {
    if (!selected || fit || view !== "map") return;
    const position = positions.get(selected.id);
    if (position && viewport.current)
      viewport.current.scrollLeft = Math.max(
        0,
        (position.x + 116) * zoom - width / 2,
      );
  }, [selectedID, width, zoom, fit, view]);
  const pick = (step: Step) => (
    <button
      ref={(element) => {
        if (element) stepButtons.current.set(step.id, element);
        else stepButtons.current.delete(step.id);
      }}
      type="button"
      className="w-full text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
      onClick={() => {
        setManualZoom(zoom);
        setFit(false);
        setSelectedID(step.id);
      }}
      aria-label={`Inspect ${step.name}`}
      aria-pressed={selectedID === step.id}
    >
      <span className="block text-sm font-semibold line-clamp-2">
        {step.name}
      </span>
      <span className="mt-2 flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
        <span>
          {step.sloRefs.length} linked SLO{step.sloRefs.length === 1 ? "" : "s"}
        </span>
        <Status value={stepHealth(step, journey)} />
      </span>
    </button>
  );
  return (
    <section
      className={expanded ? "journey-canvas-expanded min-w-0" : "min-w-0"}
      aria-label="Journey steps and dependencies"
    >
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <div className="flex gap-1" role="group" aria-label="Journey view">
          <Button
            variant={view === "map" ? "secondary" : "outline"}
            size="sm"
            aria-pressed={view === "map"}
            onClick={() => setView("map")}
          >
            Map
          </Button>
          <Button
            variant={view === "list" ? "secondary" : "outline"}
            size="sm"
            aria-pressed={view === "list"}
            onClick={() => setView("list")}
          >
            List
          </Button>
        </div>
        <div className="flex flex-wrap items-center gap-1">
          {view === "map" ? (
            <>
              <Button variant="outline" size="sm" onClick={fitJourney}>
                Fit journey
              </Button>
              <Button
                variant="ghost"
                size="icon"
                aria-label="Zoom out"
                disabled={zoom <= 0.25}
                onClick={() => changeZoom(zoom - 0.1)}
              >
                <ZoomOut className="size-4" />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => changeZoom(1)}
                aria-label="Reset journey zoom to 100 percent"
              >
                {Math.round(zoom * 100)}%
              </Button>
              <Button
                variant="ghost"
                size="icon"
                aria-label="Zoom in"
                disabled={zoom >= 1.5}
                onClick={() => changeZoom(zoom + 0.1)}
              >
                <ZoomIn className="size-4" />
              </Button>
            </>
          ) : null}
          {!expanded ? (
            <Dialog>
              <DialogTrigger asChild>
                <Button variant="outline" size="sm">
                  <Maximize2 className="size-4" />
                  Expand journey
                </Button>
              </DialogTrigger>
              <DialogContent className="h-[calc(100dvh-2rem)] w-[calc(100vw-2rem)] max-w-none overflow-auto sm:max-w-none">
                <DialogHeader>
                  <DialogTitle>
                    {journey.resource.manifest.spec.displayName}
                  </DialogTitle>
                  <DialogDescription>
                    Explore steps, dependencies and linked objectives.
                  </DialogDescription>
                </DialogHeader>
                <JourneyMap journey={journey} onSLO={onSLO} expanded />
              </DialogContent>
            </Dialog>
          ) : null}
        </div>
      </div>
      <div
        className={`grid min-w-0 gap-3${selected ? " lg:grid-cols-[minmax(0,1fr)_20rem]" : ""}`}
      >
        {view === "map" ? (
          <div
            ref={viewportRef}
            tabIndex={0}
            role="region"
            aria-label="Journey canvas. Scroll to pan, or drag empty space. Use List for an ordered view."
            className="journey-canvas-viewport min-w-0 overflow-auto rounded-md border bg-background focus-visible:outline-2 focus-visible:outline-ring"
            onPointerDown={(event) => {
              if (
                event.button !== 0 ||
                (event.target as Element).closest("button")
              )
                return;
              if (event.pointerType === "touch") return;
              drag.current = {
                x: event.clientX,
                y: event.clientY,
                left: event.currentTarget.scrollLeft,
                top: event.currentTarget.scrollTop,
              };
              event.currentTarget.setPointerCapture?.(event.pointerId);
            }}
            onPointerMove={(event) => {
              if (!drag.current) return;
              event.currentTarget.scrollLeft =
                drag.current.left - (event.clientX - drag.current.x);
              event.currentTarget.scrollTop =
                drag.current.top - (event.clientY - drag.current.y);
            }}
            onPointerUp={() => {
              drag.current = null;
            }}
            onPointerCancel={() => {
              drag.current = null;
            }}
          >
            <div
              style={{
                width: Math.max(width, contentWidth * zoom),
                height: Math.max(height, contentHeight * zoom),
                position: "relative",
              }}
            >
              <div
                data-journey-world
                style={{
                  width: contentWidth,
                  height: contentHeight,
                  position: "absolute",
                  left: Math.max(0, (width - contentWidth * zoom) / 2),
                  top: Math.max(0, (height - contentHeight * zoom) / 2),
                  transform: `scale(${zoom})`,
                  transformOrigin: "top left",
                }}
              >
                <svg
                  aria-hidden="true"
                  width={contentWidth}
                  height={contentHeight}
                  className="pointer-events-none absolute inset-0 text-muted-foreground"
                >
                  {steps.flatMap((step) =>
                    (step.dependsOn ?? []).map((dependency) => {
                      const a = positions.get(dependency),
                        b = positions.get(step.id);
                      if (!a || !b) return null;
                      return (
                        <path
                          key={`${dependency}:${step.id}`}
                          d={`M${a.x + 232},${a.y + 52} C${a.x + 268},${a.y + 52} ${b.x - 36},${b.y + 52} ${b.x},${b.y + 52}`}
                          fill="none"
                          stroke="currentColor"
                          strokeWidth={1.5}
                        />
                      );
                    }),
                  )}
                </svg>
                {layers.map((layer, index) => (
                  <span
                    key={index}
                    className="absolute text-xs text-muted-foreground"
                    style={{ left: 24 + index * 304, top: 24 }}
                  >
                    Stage {index + 1} · {layer.length} step
                    {layer.length === 1 ? "" : "s"}
                  </span>
                ))}
                {steps.map((step) => {
                  const Icon = actors[step.actor];
                  const position = positions.get(step.id)!;
                  return (
                    <div
                      key={step.id}
                      style={{
                        position: "absolute",
                        left: position.x,
                        top: position.y,
                        width: 232,
                        height: 108,
                      }}
                      className={`rounded-md border bg-card p-3${selectedID === step.id ? " border-primary ring-1 ring-primary" : ""}`}
                    >
                      <div className="mb-1 flex items-center gap-1.5 text-xs capitalize text-muted-foreground">
                        <Icon className="size-3.5" aria-hidden="true" />
                        {step.actor}
                      </div>
                      {pick(step)}
                    </div>
                  );
                })}
              </div>
            </div>
          </div>
        ) : (
          <div
            className="journey-canvas-viewport min-w-0 space-y-3 overflow-y-auto rounded-md border p-4"
            role="region"
            aria-label="Ordered journey steps"
          >
            {layers.map((layer, index) => (
              <details key={index} open className="border-b pb-3 last:border-0">
                <summary className="mb-3 text-sm font-medium">
                  Stage {index + 1} · {layer.length} step
                  {layer.length === 1 ? "" : "s"}
                </summary>
                <ol className="space-y-3">
                  {layer.map((step) => (
                    <li key={step.id} className="rounded-md border p-3">
                      {pick(step)}
                      <p className="mt-2 text-xs text-muted-foreground">
                        {step.dependsOn?.length
                          ? `After ${step.dependsOn.map((id) => steps.find((s) => s.id === id)?.name ?? id).join(", ")}`
                          : "Entry step"}
                      </p>
                    </li>
                  ))}
                </ol>
              </details>
            ))}
          </div>
        )}
        {selected ? (
          <aside
            className="journey-canvas-viewport min-w-0 overflow-y-auto rounded-md border bg-card p-4"
            aria-label="Selected journey step"
          >
            <div className="mb-3 flex items-start justify-between gap-3">
              <h3 className="break-words text-base font-semibold">
                {selected.name}
              </h3>
              <Button
                variant="ghost"
                size="icon"
                className="shrink-0"
                aria-label="Close step details"
                onClick={() => {
                  stepButtons.current.get(selected.id)?.focus();
                  setSelectedID(null);
                }}
              >
                <X className="size-4" />
              </Button>
            </div>
            <Status value={stepHealth(selected, journey)} />
            <dl className="my-4 space-y-3 text-sm">
              <div>
                <dt className="text-muted-foreground">Actor</dt>
                <dd className="capitalize">{selected.actor}</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Dependencies</dt>
                <dd>
                  {selected.dependsOn?.length
                    ? selected.dependsOn
                        .map((id) => steps.find((s) => s.id === id)?.name ?? id)
                        .join(", ")
                    : "Entry step"}
                </dd>
              </div>
            </dl>
            <h4 className="mb-3 text-sm font-semibold">Linked objectives</h4>
            {selected.sloRefs.length ? (
              <ul className="space-y-3">
                {selected.sloRefs.map((id) => {
                  const link = journey.links.find(
                    (entry) => entry.link.id === id,
                  );
                  return (
                    <li
                      key={id}
                      className="space-y-2 border-b pb-3 last:border-0"
                    >
                      <Button
                        variant="link"
                        className="h-auto max-w-full justify-start whitespace-normal break-words p-0 text-left"
                        onClick={() => onSLO(id)}
                      >
                        {link?.slo.name ?? id}
                      </Button>
                      {link ? (
                        <>
                          <p className="text-xs text-muted-foreground">
                            {link.link.required ? "Required" : "Advisory"} ·{" "}
                            {link.link.dimension}
                          </p>
                          <SLIValue
                            status={link.status}
                            target={link.slo.target}
                          />
                        </>
                      ) : (
                        <p className="text-xs text-muted-foreground">
                          Measurement unavailable
                        </p>
                      )}
                    </li>
                  );
                })}
              </ul>
            ) : (
              <p className="text-sm text-muted-foreground">
                No objectives linked to this step.
              </p>
            )}
          </aside>
        ) : null}
      </div>
      <p className="mt-3 text-xs text-muted-foreground">
        Stages follow dependency order. Steps in the same stage may run in
        parallel. Select a step to inspect its linked SLOs.{" "}
        {fit && zoom < 0.6
          ? "Use Expand journey or List for a larger, readable view."
          : ""}
      </p>
      {!steps.length ? (
        <p className="py-3 text-sm text-muted-foreground">
          No journey steps defined.
        </p>
      ) : null}
    </section>
  );
}
