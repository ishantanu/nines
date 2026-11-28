// Sign-in panel built from Astryx parts: a themed surface, Heading and Text.
// The data-slot hooks are kept for tests and session-replay privacy rules.
import { useState, type ComponentProps, type ReactNode } from "react";
import { Activity, Eye, EyeOff, GitPullRequest, Waypoints } from "lucide-react";
import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Brand } from "@/components/brand";

const STEPS = [
  {
    Icon: Waypoints,
    title: "Define the journey",
    body: "Link each user step to the objectives it depends on.",
  },
  {
    Icon: Activity,
    title: "Measure reliability",
    body: "Read SLI attainment, coverage and budget burn together.",
  },
  {
    Icon: GitPullRequest,
    title: "Review the change",
    body: "Inspect the definition and evidence before applying it.",
  },
] as const;

// Decorative only: a drifting reliability line over a soft aurora. Nothing here
// is data, so it is hidden from assistive technology and still under reduced motion.
function AuthBackdrop() {
  return (
    <div aria-hidden="true" className="auth-backdrop">
      <div className="auth-aurora" />
      <div className="auth-grid" />
      <svg
        className="auth-wave"
        viewBox="0 0 1440 900"
        preserveAspectRatio="xMidYMid slice"
      >
        <defs>
          <linearGradient id="auth-wave-fill" x1="0" x2="0" y1="0" y2="1">
            <stop offset="0" stopColor="currentColor" stopOpacity="0.22" />
            <stop offset="1" stopColor="currentColor" stopOpacity="0" />
          </linearGradient>
        </defs>
        <path
          className="auth-wave-area"
          fill="url(#auth-wave-fill)"
          d="M0 540 L90 536 L150 543 L210 531 L262 535 L298 604 L332 520 L402 528 L470 524 L522 533 L560 466 L592 541 L662 530 L722 538 L758 656 L792 556 L842 522 L922 531 L992 526 L1040 594 L1074 524 L1142 519 L1200 534 L1252 428 L1284 531 L1342 526 L1440 520 V900 H0 Z"
        />
        <path
          className="auth-wave-line"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.5"
          strokeLinejoin="round"
          pathLength="1"
          d="M0 540 L90 536 L150 543 L210 531 L262 535 L298 604 L332 520 L402 528 L470 524 L522 533 L560 466 L592 541 L662 530 L722 538 L758 656 L792 556 L842 522 L922 531 L992 526 L1040 594 L1074 524 L1142 519 L1200 534 L1252 428 L1284 531 L1342 526 L1440 520"
        />
        <path
          className="auth-wave-target"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeDasharray="3 9"
          d="M0 640 H1440"
        />
        <circle
          className="auth-wave-dot"
          cx="740"
          cy="520"
          r="6"
          fill="currentColor"
        />
      </svg>
    </div>
  );
}

export function SignInPanel({
  title = "Sign in",
  description,
  children,
  badge,
}: {
  title?: string;
  description: string;
  children: ReactNode;
  badge?: ReactNode;
}) {
  return (
    <>
      <AuthBackdrop />
      <div data-slot="sign-in" className="auth-card">
        <section className="flex min-w-0 flex-col gap-5 p-6 sm:p-9">
          <Brand className="h-8" />
          <div className="flex flex-col gap-1">
            <Heading level={1} type="display-3">
              {title}
            </Heading>
            <Text color="secondary">{description}</Text>
          </div>
          <div className="space-y-5">{children}</div>
          {badge ? <div>{badge}</div> : null}
        </section>
        <aside className="auth-aside">
          <svg
            aria-hidden="true"
            className="auth-aside-line"
            viewBox="0 0 400 120"
          >
            <path
              className="auth-aside-line-path"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.5"
              strokeLinecap="round"
              strokeLinejoin="round"
              pathLength="1"
              d="M0 70 L30 68 L55 72 L80 66 L96 102 L112 62 L150 64 L182 60 L205 24 L222 68 L262 66 L292 70 L312 108 L330 64 L362 62 L400 58"
            />
            <path
              d="M0 96 H400"
              stroke="currentColor"
              strokeOpacity="0.5"
              strokeDasharray="2 7"
            />
          </svg>
          <Heading level={2} type="display-3" className="!text-white">
            Reliability, reviewed as code.
          </Heading>
          <Text className="!text-white/80">
            Define service level objectives and critical user journeys. Review
            the measurement and evidence before applying a change.
          </Text>
          <ol className="mt-2 flex flex-col gap-4">
            {STEPS.map(({ Icon, title, body }) => (
              <li key={title} className="flex gap-3">
                <Icon
                  className="mt-0.5 size-5 shrink-0 text-white/90"
                  aria-hidden="true"
                />
                <div>
                  <Heading
                    level={3}
                    type="display-3"
                    className="!text-base !text-white"
                  >
                    {title}
                  </Heading>
                  <Text className="!text-white/75">{body}</Text>
                </div>
              </li>
            ))}
          </ol>
        </aside>
      </div>
    </>
  );
}

export function SignInPassword(
  props: Omit<ComponentProps<typeof Input>, "type">,
) {
  const [visible, setVisible] = useState(false);
  return (
    <div className="relative mt-2">
      <Input
        {...props}
        type={visible ? "text" : "password"}
        className="pr-12"
      />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="absolute right-1 top-0 h-9 w-9"
        aria-label={visible ? "Hide password" : "Show password"}
        aria-pressed={visible}
        onClick={() => setVisible((v) => !v)}
      >
        {visible ? (
          <EyeOff className="size-4" aria-hidden="true" />
        ) : (
          <Eye className="size-4" aria-hidden="true" />
        )}
      </Button>
    </div>
  );
}
