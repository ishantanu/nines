import "./site.css";
const root = document.getElementById("root")!;
if (root.hasAttribute("data-doc")) {
  void import("./docs/main").then(({ mountDocumentation }) =>
    mountDocumentation(root),
  );
} else {
  void import("./landing").then(({ mountLanding }) => mountLanding(root));
}
