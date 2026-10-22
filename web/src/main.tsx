import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./index.css";
import { startBrowserTelemetry } from "./lib/observability";
void startBrowserTelemetry().finally(() =>
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>,
  ),
);
