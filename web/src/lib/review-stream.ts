import { APIError, type RequestScope } from "./api";
import { observedFetch } from "./observability";
import type { ReviewRun } from "./investigations";
export function decodeRunEvent(
  data: string,
  scope: RequestScope,
  review: string,
  id: string,
): ReviewRun {
  const run = JSON.parse(data) as ReviewRun;
  if (
    run.apiVersion !== "nines.dev/v1alpha1" ||
    run.tenant !== scope.tenant ||
    run.id !== id ||
    run.investigationId !== review ||
    !Number.isSafeInteger(run.version) ||
    run.version < 1
  )
    throw new Error("Invalid scoped review event");
  return run;
}
// true means terminal; false requests a bounded-connection reconnect.
export async function streamReviewRun(
  scope: RequestScope,
  review: string,
  id: string,
  version: number,
  signal: AbortSignal,
  onRun: (run: ReviewRun) => void,
): Promise<boolean> {
  const headers = new Headers({
    Accept: "text/event-stream",
    "X-Nines-Tenant": scope.tenant,
    "Last-Event-ID": String(version),
  });
  if (scope.token) headers.set("Authorization", `Bearer ${scope.token}`);
  const response = await observedFetch(
    `/api/investigations/${encodeURIComponent(review)}/runs/${encodeURIComponent(id)}/events`,
    {
      method: "GET",
      headers,
      credentials: "same-origin",
      signal: AbortSignal.any([signal, scope.signal]),
    },
  );
  if (!response.ok)
    throw new APIError(response.status, "Review progress is unavailable");
  if (
    response.headers.get("Content-Type")?.split(";", 1)[0] !==
      "text/event-stream" ||
    !response.body
  )
    throw new Error("Review progress stream is unavailable");
  const reader = response.body.getReader();
  const decoder = new TextDecoder("utf-8", { fatal: true });
  let buffer = "",
    received = 0;
  try {
    while (true) {
      const chunk = await reader.read();
      if (chunk.done) break;
      received += chunk.value.byteLength;
      if (received > 8 * 1024 * 1024)
        throw new Error("Review stream exceeds bounds");
      buffer = (
        buffer + decoder.decode(chunk.value, { stream: true })
      ).replaceAll("\r\n", "\n");
      if (buffer.length > 256 * 1024)
        throw new Error("Review event exceeds bounds");
      let boundary: number;
      while ((boundary = buffer.indexOf("\n\n")) >= 0) {
        const frame = buffer.slice(0, boundary);
        buffer = buffer.slice(boundary + 2);
        let event = "",
          data = "";
        for (const line of frame.split("\n")) {
          if (line.startsWith("event:")) event = line.slice(6).trim();
          if (line.startsWith("data:"))
            data += line.slice(5).trimStart() + "\n";
        }
        if (event === "run") {
          const run = decodeRunEvent(data, scope, review, id);
          if (run.version > version) {
            version = run.version;
            onRun(run);
          }
        } else if (event === "done") {
          const reason = JSON.parse(data).reason;
          if (reason !== "terminal" && reason !== "reconnect")
            throw new Error("Invalid review completion event");
          return reason === "terminal";
        } else if (event === "access_revoked") {
          window.dispatchEvent(new CustomEvent("nines-access-changed"));
          throw new APIError(403, "Workspace access changed");
        } else if (event === "unavailable")
          throw new Error("Review progress is temporarily unavailable");
      }
    }
    throw new Error("Review progress stream ended early");
  } finally {
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}
