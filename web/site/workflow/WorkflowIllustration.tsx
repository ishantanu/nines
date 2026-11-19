import { useState } from "react";
import { FileCheck2, Terminal } from "lucide-react";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "../../src/components/ui/tabs";
import { Swap } from "./Swap";

const steps = [
  {
    id: "inspect",
    label: "Inspect",
    receipt: "Contracts available",
    command: "nines capabilities --offline --agent",
    description:
      "Discover commands and resource contracts before choosing a tool. No server or model is required.",
  },
  {
    id: "validate",
    label: "Validate",
    receipt: "Check the definition",
    command: "nines validate --offline -f slo.yaml --agent",
    description:
      "Check the YAML and measurement contract deterministically. A model recommendation must pass the same validation.",
  },
  {
    id: "plan",
    label: "Plan",
    receipt: "Inspect the proposed change",
    command: "nines plan --context production -f slo.yaml --agent",
    description:
      "Compare the definition with the selected tenant. Planning shows a proposed change without applying it.",
  },
  {
    id: "review",
    label: "Review",
    receipt: "Awaiting engineer review",
    command: "nines plan --context production -f slo.yaml --agent",
    description:
      "Review the exact plan, measurement and destination before a separately authorized write. Intent approvals are bound to the change digest.",
  },
];

/** A local illustration: selecting a step never runs a command or an API call. */
export function WorkflowIllustration() {
  const [selected, setSelected] = useState("review");
  const current = steps.find((step) => step.id === selected) ?? steps[3];
  return (
    <figure
      className="agent-terminal nines-workflow-demo"
      aria-labelledby="workflow-caption"
    >
      <figcaption id="workflow-caption" className="terminal-heading">
        <Terminal size={16} aria-hidden="true" />
        <span>A definition becomes a reviewed change</span>
      </figcaption>
      <Tabs value={selected} onValueChange={setSelected}>
        <TabsList
          className="workflow-steps"
          aria-label="Example workflow steps"
        >
          {steps.map((step, index) => (
            <TabsTrigger key={step.id} value={step.id}>
              <span className="workflow-step-number" aria-hidden="true">
                {index + 1}
              </span>
              {step.label}
            </TabsTrigger>
          ))}
        </TabsList>
        <div
          className="workflow-receipt"
          role="status"
          aria-live="polite"
          aria-atomic="true"
        >
          <FileCheck2 size={19} aria-hidden="true" />
          <Swap id={selected} duration={220}>
            {current.receipt}
          </Swap>
        </div>
        {steps.map((step) => (
          <TabsContent key={step.id} value={step.id} className="workflow-panel">
            <pre tabIndex={0} aria-label={`${step.label} command`}>
              <code>{step.command}</code>
            </pre>
            <p>{step.description}</p>
          </TabsContent>
        ))}
      </Tabs>
      <div className="workflow-boundary">
        <strong>Agents propose. Engineers decide.</strong>
        <span>
          Example only; no commands run here. Model-assisted workflows are
          experimental. Rollback cannot restore spent error budget.
        </span>
      </div>
    </figure>
  );
}
