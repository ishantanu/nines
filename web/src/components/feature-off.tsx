import { PageHeader } from "@/components/workspace-chrome";
import { MaturityBadge } from "@/components/maturity-badge";
import { maturityDocsURL, type MaturityFeature } from "@/lib/maturity";

// Shown when someone opens a page whose experimental feature this server has
// not turned on, so a bookmark explains itself instead of failing.
export function FeatureOff({ feature }: { feature: MaturityFeature }) {
  return (
    <>
      <PageHeader
        title={feature.name}
        description="This experimental feature is turned off on this server."
        badge={<MaturityBadge feature={feature} />}
      />
      <div className="empty-state rounded-md border bg-white text-sm">
        <p>
          An administrator can turn it on by adding <code>{feature.id}</code> to
          the server's <code>NINES_FEATURES</code> setting.{" "}
          <a
            href={maturityDocsURL(feature)}
            target="_blank"
            rel="noreferrer"
            className="underline"
          >
            About experimental features
          </a>
        </p>
      </div>
    </>
  );
}
