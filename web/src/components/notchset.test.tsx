// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import { AnimatedCounter } from "./ui/notchset/animated-counter";
import { MetricCard } from "./ui/notchset/metric-card";
import { TextShimmer } from "./ui/notchset/text-shimmer";
import { SignInPassword } from "./ui/notchset/sign-in";
import { DemoLogin } from "./demo-login";
import { Login } from "./login";

test("metric counts show one exact value and do not manufacture a trend", () => {
  const host = document.createElement("div");
  host.innerHTML = renderToStaticMarkup(
    <dl>
      <MetricCard label="Needs review" value={12} />
    </dl>,
  );
  expect(host.querySelector("dt")?.textContent).toBe("Needs review");
  expect(host.querySelector("dd")?.textContent).toBe("12");
  expect(host.querySelector("svg")).toBeNull();
  expect(renderToStaticMarkup(<AnimatedCounter value={NaN} />)).toContain(
    "Unavailable",
  );
});

test("working text is one readable status label and stops when inactive", () => {
  const host = document.createElement("div");
  host.innerHTML = renderToStaticMarkup(
    <TextShimmer>Preparing evidence</TextShimmer>,
  );
  expect(host.textContent).toContain("Preparing evidence");
  expect(host.querySelector('[data-slot="text-shimmer"]')).not.toBeNull();
  const idle = renderToStaticMarkup(
    <TextShimmer active={false}>Done</TextShimmer>,
  );
  expect(idle).toContain("Done");
  expect(idle).not.toContain("text-shimmer");
});

test("password visibility preserves the value and does not submit the sign-in form", async () => {
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  let submits = 0;
  await act(async () =>
    root.render(
      <form
        onSubmit={(e) => {
          e.preventDefault();
          submits++;
        }}
      >
        <label htmlFor="test-password">Password</label>
        <SignInPassword
          id="test-password"
          name="password"
          defaultValue="fixture-only"
          autoComplete="current-password"
        />
      </form>,
    ),
  );
  const field = host.querySelector("input")!;
  expect(field.type).toBe("password");
  await act(async () => host.querySelector("button")!.click());
  expect(field.type).toBe("text");
  expect(field.value).toBe("fixture-only");
  expect(field.autocomplete).toBe("current-password");
  expect(submits).toBe(0);
  expect(host.querySelector("button")?.getAttribute("aria-label")).toBe(
    "Hide password",
  );
  await act(async () => host.querySelector("button")!.click());
  expect(field.type).toBe("password");
  await act(async () => root.unmount());
  host.remove();
});

test("demo sign-in keeps labels, privacy boundaries and accessible controls", async () => {
  const host = document.createElement("div");
  document.body.append(host);
  host.innerHTML = renderToStaticMarkup(
    <DemoLogin darkMode={false} onToggleTheme={() => {}} onSignIn={() => {}} />,
  );
  expect(host.querySelector('[data-slot="sign-in"]')).not.toBeNull();
  expect(
    host.querySelector("main[data-private][data-replay-block]"),
  ).not.toBeNull();
  expect(
    host.querySelector('[data-slot="sign-in"] > aside')?.textContent,
  ).toContain("Reliability, reviewed as code.");
  expect(host.querySelector(".login-frame")).toBeNull();
  expect(host.textContent).not.toMatch(
    /EMAIL ME A CODE|CREATE ACCOUNT|Work email|Passkey/,
  );
  const { default: axe } = await import("axe-core");
  const results = await axe.run(host, {
    rules: { "color-contrast": { enabled: false } },
  });
  expect(results.violations).toEqual([]);
  host.remove();
});

test("the real sign-in block preserves configured SSO routes and hides unsupported methods", () => {
  const host = document.createElement("div");
  host.innerHTML = renderToStaticMarkup(
    <Login
      auth={{
        enabled: true,
        authenticated: false,
        csrfToken: "fixture-csrf",
        providers: [
          { id: "company", name: "Company SSO", type: "oidc" },
          { id: "local", name: "Local accounts", type: "local" },
        ],
      }}
      onSignedIn={() => {}}
      onToken={() => {}}
    />,
  );
  expect(host.querySelector('[data-slot="sign-in"] > aside')).not.toBeNull();
  expect(
    host.querySelector('a[href="/auth/oidc/company/login"]')?.textContent,
  ).toContain("Continue with Company SSO");
  expect(host.querySelector("input#login-password")?.getAttribute("type")).toBe(
    "password",
  );
  expect(host.textContent).not.toMatch(
    /EMAIL ME A CODE|CREATE ACCOUNT|Work email|Passkey/,
  );
});

test("demo account submission through the real block opens the sample workspace", async () => {
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  let signedIn = 0;
  await act(async () =>
    root.render(
      <DemoLogin
        darkMode={false}
        onToggleTheme={() => {}}
        onSignIn={() => {
          signedIn++;
        }}
      />,
    ),
  );
  await act(async () =>
    host
      .querySelector<HTMLButtonElement>('form button[type="submit"]')!
      .click(),
  );
  expect(signedIn).toBe(1);
  await act(async () => root.unmount());
  host.remove();
});

test("measured counters render the exact formatted precision", async () => {
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  const format = (n: number) => `${(n * 100).toFixed(3)}% SLI`;
  try {
    await act(async () =>
      root.render(<AnimatedCounter value={0.9995} format={format} />),
    );
    expect(host.textContent).toBe("99.950% SLI");
    await act(async () =>
      root.render(<AnimatedCounter value={0.9996} format={format} />),
    );
    expect(host.textContent).toBe("99.960% SLI");
  } finally {
    await act(async () => root.unmount());
    host.remove();
  }
});
