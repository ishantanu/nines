const projectToken = import.meta.env.VITE_POSTHOG_PROJECT_TOKEN?.trim() ?? "";
const configuredHost = import.meta.env.VITE_POSTHOG_HOST?.trim() ?? "";

function validHost(value: string): boolean {
  try {
    const host = new URL(value);
    const isLocalHost = ["localhost", "127.0.0.1", "[::1]"].includes(
      host.hostname,
    );
    return (
      ["http:", "https:"].includes(host.protocol) &&
      (host.protocol === "https:" || isLocalHost) &&
      !host.username &&
      !host.password &&
      !host.search &&
      !host.hash &&
      host.pathname === "/"
    );
  } catch {
    return false;
  }
}

export function isSessionReplayConfigured(): boolean {
  return projectToken.length > 0 && validHost(configuredHost);
}

function stripUrlParameters(value: string): string {
  try {
    const url = new URL(value, window.location.origin);
    return value.startsWith("/")
      ? url.pathname
      : `${url.protocol}//${url.host}${url.pathname}`;
  } catch {
    return "";
  }
}

function stripSensitiveUrlFields(value: unknown): void {
  if (Array.isArray(value)) {
    value.forEach(stripSensitiveUrlFields);
    return;
  }
  if (typeof value !== "object" || value === null) return;
  for (const [key, nested] of Object.entries(value)) {
    if (typeof nested === "string" && /(?:url|href|uri)$/i.test(key)) {
      Reflect.set(value, key, stripUrlParameters(nested));
    } else {
      stripSensitiveUrlFields(nested);
    }
  }
}

type ReplayClient = Pick<
  typeof import("posthog-js").default,
  "init" | "startSessionRecording" | "stopSessionRecording"
>;

// One owner at a time. A stale import/cleanup cannot start or stop a newer session.
export function replayController(
  load: () => Promise<ReplayClient>,
  initialize: (client: ReplayClient) => void,
) {
  let generation = 0;
  let activeStop: (() => void) | undefined;
  let client: ReplayClient | undefined;
  return async (signal: AbortSignal): Promise<() => void> => {
    activeStop?.();
    const owner = ++generation;
    let started = false;
    const stop = () => {
      signal.removeEventListener("abort", stop);
      if (generation !== owner) return;
      generation++;
      if (started) client?.stopSessionRecording();
    };
    signal.addEventListener("abort", stop, { once: true });
    if (signal.aborted) {
      stop();
      return () => {};
    }
    try {
      const loaded = await load();
      if (signal.aborted || generation !== owner) {
        signal.removeEventListener("abort", stop);
        return () => {};
      }
      if (!client) {
        initialize(loaded);
        client = loaded;
      }
      if (signal.aborted || generation !== owner) return () => {};
      client.startSessionRecording();
      started = true;
      activeStop = stop;
      return stop;
    } catch (error) {
      stop();
      throw error;
    }
  };
}
const start = replayController(
  async () => (await import("posthog-js/full/no-external")).default,
  (posthog) =>
    posthog.init(projectToken, {
      api_host: configuredHost,
      autocapture: false,
      capture_pageview: false,
      capture_pageleave: false,
      capture_exceptions: false,
      disable_capture_url_hashes: true,
      disable_session_recording: true,
      disable_external_dependency_loading: true,
      disable_compression: true,
      persistence: "memory",
      save_referrer: false,
      save_campaign_params: false,
      before_send: (event) => {
        stripSensitiveUrlFields(event.properties);
        if (event.$set) stripSensitiveUrlFields(event.$set);
        if (event.$set_once) stripSensitiveUrlFields(event.$set_once);
        return event;
      },
      session_recording: {
        maskAllInputs: true,
        maskTextSelector: "*",
        maskAllElementAttributes: true,
        recordCrossOriginIframes: false,
        recordHeaders: false,
        recordBody: false,
        maskCapturedNetworkRequestFn: (request) => {
          if (request.name) request.name = request.name.split(/[?#]/, 1)[0];
          return request;
        },
      },
    }),
);
export async function startDemoSessionReplay(
  signal: AbortSignal,
): Promise<() => void> {
  if (!isSessionReplayConfigured())
    throw new Error("Session replay is not configured for this deployment.");
  return start(signal);
}
