import { useId, useState } from "react";
import { CalendarDays, ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { duration } from "@/lib/history";
import {
  absoluteRange,
  rangePresets,
  utcInput,
  type ChartRange,
} from "@/lib/chart-range";
import { ReplayLabel } from "@/components/replay-label";

export function ChartTimeRange({
  value,
  maxSeconds,
  stepSeconds,
  observedTo,
  onChange,
}: {
  value: ChartRange;
  maxSeconds: number;
  stepSeconds: number;
  observedTo?: string;
  onChange: (range: ChartRange) => void;
}) {
  const id = useId();
  const [open, setOpen] = useState(false);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [error, setError] = useState("");
  function openPicker(next: boolean) {
    if (next) {
      const last = observedTo ? Date.parse(observedTo) : Date.now();
      const end = Math.min(
        Number.isFinite(last) ? last : Date.now(),
        Date.now(),
      );
      setFrom(
        utcInput(
          value.kind === "absolute"
            ? value.from
            : new Date(
                end - Math.min(value.seconds, maxSeconds) * 1000,
              ).toISOString(),
        ),
      );
      setTo(
        utcInput(
          value.kind === "absolute" ? value.to : new Date(end).toISOString(),
        ),
      );
      setError("");
    }
    setOpen(next);
  }
  const label =
    value.kind === "relative"
      ? `Last ${duration(Math.min(value.seconds, maxSeconds))}`
      : "Custom range";
  return (
    <div className="min-w-0">
      <Label htmlFor={id}>
        <ReplayLabel>Time range</ReplayLabel>
      </Label>
      <Popover open={open} onOpenChange={openPicker}>
        <PopoverTrigger asChild>
          <Button
            id={id}
            variant="outline"
            className="mt-1 w-full justify-between sm:min-w-44"
            aria-label={`Time range: ${label}`}
          >
            <CalendarDays />
            <span>{label}</span>
            <ChevronDown />
          </Button>
        </PopoverTrigger>
        <PopoverContent aria-label="Select chart time range" className="p-3">
          <h3 className="font-semibold">Time range</h3>
          <p className="mt-1 text-xs text-muted-foreground">
            Dates and chart axes use UTC. Up to {duration(maxSeconds)} of
            retained history.
          </p>
          <div
            className="my-3 grid grid-cols-3 gap-2"
            aria-label="Quick ranges"
          >
            {rangePresets(maxSeconds).map((seconds) => (
              <Button
                key={seconds}
                size="sm"
                variant={
                  value.kind === "relative" &&
                  Math.min(value.seconds, maxSeconds) === seconds
                    ? "secondary"
                    : "outline"
                }
                onClick={() => {
                  onChange({ kind: "relative", seconds });
                  setOpen(false);
                }}
              >
                Last {duration(seconds)}
              </Button>
            ))}
          </div>
          <form
            className="space-y-3 border-t pt-3"
            onSubmit={(event) => {
              event.preventDefault();
              try {
                const next = absoluteRange(from, to, stepSeconds, maxSeconds);
                onChange(next);
                setOpen(false);
              } catch (e) {
                setError(
                  e instanceof Error ? e.message : "Choose a valid time range.",
                );
              }
            }}
          >
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="min-w-0 space-y-2">
                <Label htmlFor={`${id}-from`}>From (UTC)</Label>
                <Input
                  id={`${id}-from`}
                  type="datetime-local"
                  step="1"
                  value={from}
                  onChange={(e) => {
                    setFrom(e.target.value);
                    setError("");
                  }}
                  aria-invalid={!!error}
                  aria-describedby={error ? `${id}-error` : undefined}
                />
              </div>
              <div className="min-w-0 space-y-2">
                <Label htmlFor={`${id}-to`}>To (UTC)</Label>
                <Input
                  id={`${id}-to`}
                  type="datetime-local"
                  step="1"
                  value={to}
                  onChange={(e) => {
                    setTo(e.target.value);
                    setError("");
                  }}
                  aria-invalid={!!error}
                  aria-describedby={error ? `${id}-error` : undefined}
                />
              </div>
            </div>
            <p className="text-xs text-muted-foreground">
              Dates align to complete {duration(stepSeconds)} intervals. Changes
              load only when applied.
            </p>
            {error ? (
              <p
                id={`${id}-error`}
                role="alert"
                className="text-sm text-destructive"
              >
                {error}
              </p>
            ) : null}
            <div className="flex justify-end gap-2">
              <Button
                size="sm"
                type="button"
                variant="outline"
                onClick={() => setOpen(false)}
              >
                Cancel
              </Button>
              <Button size="sm" type="submit">
                Apply range
              </Button>
            </div>
          </form>
        </PopoverContent>
      </Popover>
    </div>
  );
}
