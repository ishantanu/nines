import { Link } from "@astryxdesign/core/Link";
import { Badge } from "../../src/components/ui/badge";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "../../src/components/ui/tabs";
import { SLO_TYPES } from "../../src/lib/slo-types";
import availability from "../../../examples/request-success.slo.yaml?raw";
import latency from "../../../examples/request-latency.slo.yaml?raw";

// Source-backed: the availability and latency definitions are the shipped
// examples, and the rest are the starter definitions the portal shows.
const definitions: Record<string, { file: string; yaml: string }> = {
  availability: { file: "request-success.slo.yaml", yaml: availability },
  latency: { file: "request-latency.slo.yaml", yaml: latency },
};
// The same split as the portal and the SLO types guide: these are what an
// SLO counts. Per-customer reports are an add-on to any of them, and custom
// queries are the fallback, so neither is a tab.
const tabs = SLO_TYPES.filter((t) => t.id !== "custom" && t.id !== "customers");

export function SLOTypes() {
  return (
    <section
      className="container product-section slo-types"
      id="slo-types"
      aria-labelledby="slo-types-title"
    >
      <div className="section-heading">
        <h2 id="slo-types-title">One platform, every kind of SLO.</h2>
        <p>
          Every SLI kind from the SRE book: availability and latency for
          serving, durability for storage, and freshness, coverage, correctness
          and throughput for pipelines. Sessions, good-minute intervals and
          agent journey runs cover what a plain event ratio hides. Per-customer
          reports, labels and budget response levels add to any of them.
        </p>
      </div>
      <Tabs defaultValue="availability" className="definition-panel">
        <div className="definition-toolbar">
          <TabsList aria-label="SLO types">
            {tabs.map((t) => (
              <TabsTrigger key={t.id} value={t.id}>
                {t.name}
              </TabsTrigger>
            ))}
          </TabsList>
        </div>
        {tabs.map((t) => {
          const def = definitions[t.id];
          return (
            <TabsContent key={t.id} value={t.id}>
              <div className="slo-type-detail">
                <div className="slo-type-copy">
                  <p className="slo-type-badges">
                    <Badge variant={def ? "default" : "secondary"}>
                      {t.definedIn}
                    </Badge>
                  </p>
                  <h3>What it counts</h3>
                  <p>{t.measures}</p>
                  <h3>Choose it when</h3>
                  <p>{t.useWhen}</p>
                  <Link href={"./docs/" + t.docs}>Read the guide</Link>
                </div>
                <div>
                  <div className="file-name">
                    {def ? def.file : "Starter definition"}
                  </div>
                  <pre tabIndex={0} aria-label={`${t.name} definition example`}>
                    <code>{def ? def.yaml : t.snippet}</code>
                  </pre>
                </div>
              </div>
            </TabsContent>
          );
        })}
      </Tabs>
      <p className="slo-types-more">
        Compare them all in <Link href="./docs/slo-types/">SLO types</Link>,
        including{" "}
        <Link href="./docs/slo-types/#which-providers-support-which-types">
          which providers support each type
        </Link>
        . Already use Sloth or Pyrra?{" "}
        <Link href="./docs/migration/">Import your SLOs</Link>, or{" "}
        <Link href="./docs/git-delivery/">deliver new ones through Git</Link>.
      </p>
    </section>
  );
}
