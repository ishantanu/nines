import { Bot, UserRound, Server, ShieldCheck, ArrowRight } from "lucide-react";
import type { Journey } from "@/lib/api";
import { stepLayers } from "@/lib/api";
import { SLIValue } from "./sli-value";
const actors = {
  agent: Bot,
  user: UserRound,
  service: Server,
  human: ShieldCheck,
};
export function JourneyMap({
  journey,
  onSLO,
}: {
  journey: Journey;
  onSLO: (id: string) => void;
}) {
  const layers = stepLayers(journey.resource.manifest.spec.steps);
  return (
    <div className="journey-map overflow-x-auto rounded-md border bg-slate-50/60 p-4">
      <div
        className="flex min-w-max items-center gap-3"
        aria-label="Journey steps and dependencies"
      >
        {layers.map((layer, index) => (
          <div key={layer[0].id} className="flex items-center gap-3">
            {index > 0 ? (
              <ArrowRight
                className="size-4 shrink-0 text-slate-400"
                aria-hidden="true"
              />
            ) : null}
            <div className="flex flex-col gap-3">
              {layer.map((step) => {
                const Icon = actors[step.actor];
                return (
                  <div
                    key={step.id}
                    className="w-56 rounded-md border bg-white p-3 shadow-none"
                  >
                    <div className="mb-3 flex items-center justify-between">
                      <span className="rounded bg-slate-100 p-1.5">
                        <Icon className="size-4 text-slate-600" />
                      </span>
                      <span className="text-xs capitalize text-muted-foreground">
                        {step.actor}
                      </span>
                    </div>
                    <div className="text-sm font-semibold">{step.name}</div>
                    {step.dependsOn?.length ? (
                      <div className="mt-1 text-xs text-muted-foreground">
                        After {step.dependsOn.join(", ")}
                      </div>
                    ) : null}
                    <div className="mt-3 space-y-2">
                      {step.sloRefs.map((id) => (
                        <button
                          key={id}
                          onClick={() => onSLO(id)}
                          className="flex w-full items-center justify-between gap-1 text-left text-xs text-slate-600 hover:text-primary"
                        >
                          <span className="truncate underline decoration-slate-300 underline-offset-4">
                            {id}
                          </span>
                          <span className="sr-only">Open SLO</span>
                          <SLIValue
                            compact
                            status={
                              journey.links.find((x) => x.link.id === id)
                                ?.status ?? {
                                reliability: null,
                                dataStatus: "no_data",
                              }
                            }
                          />
                        </button>
                      ))}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        ))}
      </div>
      <p className="mt-4 text-xs text-muted-foreground">
        Steps in the same column may run in parallel. Each step lists its
        explicit dependencies.
      </p>
    </div>
  );
}
