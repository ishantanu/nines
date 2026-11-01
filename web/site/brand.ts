import wordmark from "../../docs/public/brand/nines.svg?raw";

// Tiny canonical artwork travels with the static HTML, avoiding a separate
// image round trip on every cold documentation visit.
export const inlineWordmark = `data:image/svg+xml,${encodeURIComponent(wordmark)}`;
