import { renderToString } from "react-dom/server";
import { App } from "./App";
export function renderSite() {
  return renderToString(<App />);
}
