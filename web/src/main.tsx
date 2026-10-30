import { createRoot } from "react-dom/client";
import { frontendModules } from "./app/modules";
import { handleCaughtError } from "./app/providers/caught-error";
import { Providers } from "./app/providers/providers";

const root = document.getElementById("root");
if (root) {
  createRoot(root, { onCaughtError: handleCaughtError }).render(
    <Providers modules={frontendModules} />,
  );
}
