import { hydrateRoot } from "react-dom/client";
import { App } from "./App";
import "./site.css";

hydrateRoot(document.getElementById("root")!, <App />);
