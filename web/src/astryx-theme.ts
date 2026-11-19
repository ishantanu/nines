import { defineTheme } from "@astryxdesign/core/theme";
import { neutralTheme } from "@astryxdesign/theme-neutral/built";

// Astryx's font CSS pulls Figtree from Google Fonts, which the server's CSP
// blocks. index.css self-hosts the same face from web/site/fonts;
// fonts are the consumer's to load.
export const ninesTheme = defineTheme({
  name: "nines",
  extends: neutralTheme,
  color: { accent: ["#62508f", "#b5a1df"] },
  typography: {
    body: {
      family: "Figtree",
      fallbacks:
        'ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
    },
    heading: {
      family: "Figtree",
      fallbacks:
        'ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
    },
  },
});
