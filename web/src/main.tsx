import { createRoot } from "react-dom/client";
import { frontendModules } from "./app/modules";
import { Providers } from "./app/providers/providers";

const root = document.getElementById("root");
if (root) {
  createRoot(root).render(<Providers modules={frontendModules} />);
}
