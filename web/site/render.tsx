import { renderToString } from "react-dom/server";
import { App } from "./App";
export { docPages } from "./docs/navigation";
export {
  documentationHTML,
  loadDocumentationSource,
} from "./docs/ServerDocument";
export function renderSite() {
  return renderToString(<App />);
}
