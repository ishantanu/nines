import { validateReplayEvents } from "./lib/replay-validation";
import { Replayer } from "@rrweb/replay";
import playerCSS from "@rrweb/replay/dist/style.css?inline";
const sheet = document.createElement("style");
sheet.textContent = playerCSS;
document.head.appendChild(sheet);
let player: Replayer | undefined;
const root = document.getElementById("player")!;
const status = document.getElementById("status")!;
const play = document.getElementById("play") as HTMLButtonElement;
const pause = document.getElementById("pause") as HTMLButtonElement;
const seek = document.getElementById("seek") as HTMLInputElement;
let duration = 0;
let lastOffset = 0;
function fit() {
  if (!player) return;
  const wrapper = root.querySelector(".replayer-wrapper") as HTMLElement | null;
  const frame = root.querySelector("iframe");
  if (!wrapper || !frame) return;
  const width =
    parseFloat(frame.style.width) ||
    Number(frame.getAttribute("width")) ||
    1024;
  const height =
    parseFloat(frame.style.height) ||
    Number(frame.getAttribute("height")) ||
    768;
  const scale = Math.min(
    root.clientWidth / width,
    root.clientHeight / height,
    1,
  );
  wrapper.style.transformOrigin = "top left";
  wrapper.style.transform = `scale(${scale})`;
  wrapper.style.marginLeft = `${Math.max(0, (root.clientWidth - width * scale) / 2)}px`;
}
function report(state: "ready" | "error") {
  if (parent !== window)
    parent.postMessage({ type: "nines-replay-state", state }, location.origin);
}
function fail() {
  const previous = player;
  player = undefined;
  previous?.destroy();
  root.replaceChildren();
  status.textContent = "This recording could not be played.";
  play.disabled = true;
  pause.disabled = true;
  seek.disabled = true;
  report("error");
}
function load(events: unknown) {
  try {
    validateReplayEvents(events);
    const previous = player;
    player = undefined;
    previous?.destroy();
    root.replaceChildren();
    player = new Replayer(events as ConstructorParameters<typeof Replayer>[0], {
      root,
      showWarning: false,
      showDebug: false,
      mouseTail: false,
      blockClass: "rr-block",
      UNSAFE_replayCanvas: false,
    });
    duration = player.getMetaData().totalTime;
    seek.max = String(Math.max(duration, 1));
    seek.value = "0";
    lastOffset = 0;
    player.pause(0);
    status.textContent = "Ready";
    play.disabled = false;
    pause.disabled = false;
    seek.disabled = false;
    player.on("resize", fit);
    fit();
    report("ready");
  } catch {
    fail();
  }
}
window.addEventListener("message", (event) => {
  if (
    event.source === parent &&
    parent !== window &&
    event.origin === location.origin &&
    event.data?.type === "nines-replay-load"
  )
    load(event.data.events);
});
const embedded = document.getElementById("replay-events");
if (embedded) {
  try {
    load(JSON.parse(embedded.textContent ?? "[]"));
  } catch {
    fail();
  }
}
play.addEventListener("click", () => {
  if (!player) return;
  player.play(lastOffset);
  status.textContent = "Playing";
});
pause.addEventListener("click", () => {
  if (!player) return;
  lastOffset = player.getCurrentTime();
  player.pause();
  status.textContent = "Paused";
});
seek.addEventListener("input", () => {
  if (!player) return;
  lastOffset = Number(seek.value);
  player.pause(lastOffset);
  status.textContent = "Paused";
});
setInterval(() => {
  if (player) {
    const now = player.getCurrentTime();
    if (Number.isFinite(now)) {
      seek.value = String(Math.min(now, duration));
      if (now >= duration) {
        lastOffset = 0;
        status.textContent = "Finished";
      }
    }
  }
}, 250);
window.addEventListener("resize", fit);
window.addEventListener("pagehide", () => player?.destroy());
if (parent !== window)
  parent.postMessage({ type: "nines-replay-ready" }, location.origin);
new ResizeObserver(fit).observe(root);
