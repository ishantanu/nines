import { createRoot, hydrateRoot } from "react-dom/client";
import { App } from "./App";

export function mountLanding(root: HTMLElement) {
  if (root.hasChildNodes()) hydrateRoot(root, <App />);
  else createRoot(root).render(<App />);
}
