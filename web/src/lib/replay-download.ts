import { validateReplayEvents } from "./replay-validation";
import playerHTML from "../../replay-player.html?raw";
import type { ReplayEvent } from "./replay-capture";

export function portableReplayHTML(
  events: ReplayEvent[],
  playerBundle: string,
  nonce: string,
) {
  if (!/^[a-f0-9]{32}$/.test(nonce)) throw Error("Invalid player nonce");
  validateReplayEvents(events);
  // Only the shipped player is executable. Recording data cannot close its JSON container.
  const payload = JSON.stringify(events).replace(/</g, "\\u003c");
  if (payload.length > 3 * 1024 * 1024 || !playerBundle.trim())
    throw Error("Invalid portable replay size or player");
  const policy = `default-src 'none'; script-src 'nonce-${nonce}'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; frame-src 'self'; form-action 'none'; object-src 'none'; base-uri 'none'`;
  return playerHTML
    .replace(
      '<meta charset="UTF-8" />',
      `<meta charset="UTF-8" /><meta http-equiv="Content-Security-Policy" content="${policy}">`,
    )
    .replace(
      /<script type="module"[\s\S]*?<\/script>/,
      () =>
        `<script type="application/json" id="replay-events" nonce="${nonce}">${payload}</script><script nonce="${nonce}">${playerBundle.replace(/<\/script/gi, "<\\/script")}</script>`,
    );
}

export async function downloadReplay(
  id: string,
  events: ReplayEvent[],
  signal: AbortSignal,
) {
  const response = await fetch("/replay-player-offline.js", {
    signal,
    cache: "force-cache",
  });
  if (!response.ok)
    throw Error("The offline replay player could not be loaded");
  const bundle = await response.text();
  if (signal.aborted) return;
  const nonce = Array.from(crypto.getRandomValues(new Uint8Array(16)), (v) =>
    v.toString(16).padStart(2, "0"),
  ).join("");
  const url = URL.createObjectURL(
    new Blob([portableReplayHTML(events, bundle, nonce)], {
      type: "text/html;charset=utf-8",
    }),
  );
  const link = document.createElement("a");
  link.href = url;
  link.download = `nines-replay-${id.replace(/[^a-z0-9-]/gi, "-")}.html`;
  document.body.appendChild(link);
  try {
    link.click();
  } finally {
    link.remove();
  }
  // Allow the browser to consume the blob before releasing it.
  setTimeout(() => URL.revokeObjectURL(url), 60000);
}
