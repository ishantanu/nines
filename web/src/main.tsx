import React from "react";
import ReactDOM from "react-dom/client";
import "./index.css"; // first: declares the cascade-layer order
import App from "./App";
import { AccountSetup } from "./components/account-setup";
import { startBrowserTelemetry } from "./lib/observability";
void startBrowserTelemetry().finally(() =>
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      {/* A setup link is opened without a session, outside the app's routing. */}
      {window.location.pathname === "/setup" ? <AccountSetup /> : <App />}
    </React.StrictMode>,
  ),
);
