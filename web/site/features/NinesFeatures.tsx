import { FeatureBento } from "../../src/components/ui/notchset/blocks/feature-bento";

export function NinesFeatures() {
  return (
    <FeatureBento
      id="features"
      className="container nines-feature-bento"
      title="From measurement to reviewed change"
      description="Interactive illustrations with synthetic data. Guide links open documentation; these examples never apply a change."
      review={{
        title: "Review the exact change",
        body: "Keep the goal, confirmed measurement, evidence and proposed revisions together for review.",
        trace: "Review schematic · not live",
        people: ["Engineer", "Reviewer"],
        initials: ["EN", "RE"],
        note: "Confirm which requests count.",
        lines: [
          { text: "Eligible requests", time: "defined" },
          { text: "Target and window", time: "reviewed" },
          { text: "Measurement", time: "confirm" },
        ],
      }}
      palette={{
        title: "Work without a dashboard",
        body: "Open the guides for the CLI, MCP and intent preparation. Search or use the arrow keys and Enter.",
        commands: [
          {
            glyph: "I",
            label: "Intent preparation",
            kind: "GUIDE",
            result: "Read the intent guide",
            href: "./docs/intents/",
          },
          {
            glyph: "C",
            label: "CLI workflows",
            kind: "GUIDE",
            result: "Read the CLI guide",
            href: "./docs/cli/",
          },
          {
            glyph: "M",
            label: "MCP integration",
            kind: "GUIDE",
            result: "Read the MCP guide",
            href: "./docs/mcp/",
          },
        ],
      }}
      cost={{
        title: "Inspect budget burn",
        body: "Focus or hover a sample interval to read its burn rate. These values are illustrative, not live telemetry.",
        label: "Synthetic budget burn",
        baseline: 1,
        comparisonLabel: "Sample",
        formatValue: (value) => `${value.toFixed(2)}×`,
        weeks: [
          { label: "00:00 UTC", value: 0.2 },
          { label: "00:05 UTC", value: 0.4 },
          { label: "00:10 UTC", value: 1.2 },
          { label: "00:15 UTC", value: 0.8 },
          { label: "00:20 UTC", value: 0.6 },
          { label: "00:25 UTC", value: 0.4 },
        ],
      }}
      automation={{
        title: "Apply after approval",
        body: "Agents prepare recommendations. Authorized engineers review the exact digest, and a separate identity applies it.",
        label: "Review sequence",
        steps: [
          { kind: "PREPARE", text: "Freeze the goal and evidence" },
          { kind: "REVIEW", text: "Approve the exact change digest" },
          { kind: "APPLY", text: "Use an independent authorized identity" },
        ],
        footnote: "Illustration only. No SLO or rule is changed.",
      }}
    />
  );
}
