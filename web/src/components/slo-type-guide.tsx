import { useState } from "react";
import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
import { Link } from "@astryxdesign/core/Link";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DOCS_BASE, SLO_TYPES, type SLOType } from "@/lib/slo-types";

export type GuideChoice =
  "availability" | "latency" | "sessions" | "good-minute" | "customers";
const actionLabel: Partial<Record<string, string>> = {
  availability: "Use availability",
  latency: "Use latency",
  sessions: "Use sessions",
  "good-minute": "Use good-minute",
  customers: "Add per-customer report",
};

// The kinds of SLO, side by side. Each kind has a guided form: choosing one
// switches the form below (or the Settings tab for good-minute). The YAML
// equivalent stays available for review and automation.
export function SLOTypeGuide({
  selected,
  customersEnabled = false,
  onChoose,
  disabled = false,
  title = "Choose the kind of SLO",
}: {
  selected?: string;
  customersEnabled?: boolean;
  onChoose?: (kind: GuideChoice) => void;
  disabled?: boolean;
  title?: string;
}) {
  const [copied, setCopied] = useState("");
  async function copy(type: SLOType) {
    try {
      await navigator.clipboard.writeText(type.snippet ?? "");
      setCopied(type.id);
    } catch {
      setCopied("");
    }
  }
  return (
    <section aria-label="SLO types" className="space-y-3">
      <div>
        <Heading level={3}>{title}</Heading>
        <Text as="p" size="sm" color="secondary">
          Each kind counts something different. Pick the one that matches what
          your users experience.
        </Text>
      </div>
      <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {SLO_TYPES.filter((t) => t.id !== "custom").map((t) => {
          const choice = actionLabel[t.id] !== undefined;
          const active =
            selected === t.id || (t.id === "customers" && customersEnabled);
          return (
            <li
              key={t.id}
              className="flex min-w-0 flex-col gap-2 rounded-md border p-3"
              data-active={active || undefined}
              data-slo-type-tile={t.id}
            >
              <div className="flex flex-wrap items-center gap-2">
                <Heading level={4}>{t.name}</Heading>
                <Badge variant="default">{t.definedIn}</Badge>
              </div>
              <Text as="p" size="sm">
                {t.measures}
              </Text>
              <Text as="p" size="sm" color="secondary">
                {t.useWhen}
              </Text>
              {choice && onChoose ? (
                <Button
                  type="button"
                  size="sm"
                  variant={active ? "default" : "outline"}
                  aria-pressed={active}
                  disabled={disabled}
                  onClick={() => onChoose(t.id as GuideChoice)}
                >
                  {active ? "Selected" : actionLabel[t.id]}
                </Button>
              ) : t.id === "journey" ? (
                <Text as="p" size="sm" color="secondary">
                  Define it on Critical user journeys, in an agentic journey.
                </Text>
              ) : null}
              {t.snippet ? (
                <details className="text-sm">
                  <summary className="cursor-pointer">
                    YAML equivalent for {t.name.toLowerCase()}
                  </summary>
                  <pre
                    tabIndex={0}
                    className="mt-2 overflow-x-auto rounded-md bg-muted p-2 text-xs"
                  >
                    {t.snippet}
                  </pre>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => copy(t)}
                  >
                    {copied === t.id ? "Copied" : "Copy"}
                  </Button>
                </details>
              ) : null}
              <Link href={DOCS_BASE + t.docs} isExternalLink>
                Read the guide
              </Link>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
