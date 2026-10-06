import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
// Fonts are bundled and served by the app itself: loading them from a font CDN
// would tell a third party about every visitor who opens a share.
import "@fontsource-variable/geist";
import "@fontsource-variable/geist-mono";
import "./index.css";
import App from "./App.tsx";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

// The worker keeps text shared into the installed app off the wire; see
// public/sw.js. It caches nothing, so a failed registration costs nothing.
if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js").catch(() => {});
  });
}
