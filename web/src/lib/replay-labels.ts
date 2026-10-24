// One finite vocabulary is shared with the independent Go sanitizer.
// Never populate this catalog with resource names, provider values or user input.
import labels from "../../../internal/replay/public-labels.json";
export const replayLabels = new Set<string>(labels);
export const replayPolicy = "selective-v2";
export function publicReplayText(text: string, label = "") {
  return replayLabels.has(label) && text.trim() === label
    ? text
    : text.slice(0, 20000).replace(/\S/gu, "*");
}
