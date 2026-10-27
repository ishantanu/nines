import type { ReactNode } from "react";
import { Activity, GitPullRequest, Waypoints } from "lucide-react";
import { Brand } from "./brand";

export function LoginFrame({ children }: { children: ReactNode }) {
  return (
    <div className="login-frame">
      <aside className="login-story" aria-labelledby="login-story-title">
        <Brand className="h-9" />
        <h2 id="login-story-title">Connect reliability to the user journey.</h2>
        <p>
          Service level objectives and critical user journeys, defined together
          and reviewed as code.
        </p>
        <ol className="login-workflow">
          <li>
            <Waypoints aria-hidden="true" />
            <div>
              <h3>Define the journey</h3>
              <p>Link each user step to the objectives it depends on.</p>
            </div>
          </li>
          <li>
            <Activity aria-hidden="true" />
            <div>
              <h3>Measure reliability</h3>
              <p>Read SLI attainment, coverage and budget burn together.</p>
            </div>
          </li>
          <li>
            <GitPullRequest aria-hidden="true" />
            <div>
              <h3>Review the change</h3>
              <p>Inspect the definition and evidence before applying it.</p>
            </div>
          </li>
        </ol>
      </aside>
      <div className="login-form-surface">{children}</div>
    </div>
  );
}
