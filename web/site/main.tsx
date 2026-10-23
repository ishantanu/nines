import { createRoot, hydrateRoot } from "react-dom/client";
import "./site.css";
const root = document.getElementById("root")!;
if (root.hasAttribute("data-doc")) {
  void import("./docs/main").then(({ mountDocumentation }) =>
    mountDocumentation(root),
  );
} else {
  void import("./App").then(({ App }) => {
    if (root.hasChildNodes()) hydrateRoot(root, <App />);
    else createRoot(root).render(<App />);
  });
}
