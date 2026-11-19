import { useId, useState } from "react";
import { CalendarDays, ChevronDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover } from "@astryxdesign/core/Popover";
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
  const presets = [
    ...new Set([
      ...rangePresets(maxSeconds, stepSeconds),
      300,
      900,
      1800,
      3600,
      10800,
      21600,
      43200,
      86400,
      604800,
      2419200,
    ]),
  ].sort((a, b) => a - b);
  const label =
    value.kind === "relative"
      ? `Last ${duration(Math.min(value.seconds, maxSeconds))}`
      : "Custom range";
  return (
    <div className="min-w-0">
      <Label htmlFor={id}>
        <ReplayLabel>Time range</ReplayLabel>
      </Label>
      <Popover
        isOpen={open}
        onOpenChange={openPicker}
        label="Select chart time range"
        padding={4}
        content={
          <>
            <h3 className="font-semibold">Time range</h3>
            <p
              id={`${id}-limit`}
              className="mt-1 text-sm text-muted-foreground"
            >
              History limit: {duration(maxSeconds)}. All times UTC.
            </p>
            <div className="mt-4 grid gap-4 sm:grid-cols-[10rem_minmax(0,1fr)]">
              <div
                className="max-h-64 space-y-1 overflow-y-auto pr-1 sm:max-h-80"
                aria-label="Quick ranges"
                aria-describedby={`${id}-limit`}
              >
                {presets.map((seconds) => (
                  <Button
                    key={seconds}
                    size="sm"
                    className="w-full justify-start"
                    disabled={seconds > maxSeconds || seconds < stepSeconds}
                    aria-pressed={
                      value.kind === "relative" &&
                      Math.min(value.seconds, maxSeconds) === seconds
                    }
                    title={
                      seconds > maxSeconds
                        ? "Outside retained history"
                        : seconds < stepSeconds
                          ? "Shorter than one measurement interval"
                          : undefined
                    }
                    variant={
                      value.kind === "relative" &&
                      Math.min(value.seconds, maxSeconds) === seconds
                        ? "secondary"
                        : "ghost"
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
                className="min-w-0 space-y-3 border-t pt-4 sm:border-t-0 sm:border-l sm:pt-0 sm:pl-4"
                onSubmit={(event) => {
                  event.preventDefault();
                  try {
                    const next = absoluteRange(
                      from,
                      to,
                      stepSeconds,
                      maxSeconds,
                    );
                    onChange(next);
                    setOpen(false);
                  } catch (e) {
                    setError(
                      e instanceof Error
                        ? e.message
                        : "Choose a valid time range.",
                    );
                  }
                }}
              >
                <h4 className="font-medium">Custom range</h4>
                <div className="grid gap-3">
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
                <p className="text-sm text-muted-foreground">
                  Dates align to complete {duration(stepSeconds)} intervals.
                  Changes load only when applied.
                </p>
                {error ? (
                  <p
                    id={`${id}-error`}
                    role="alert"
                    className="text-sm text-[var(--status-degraded)]"
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
            </div>
          </>
        }
      >
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
      </Popover>
    </div>
  );
}
