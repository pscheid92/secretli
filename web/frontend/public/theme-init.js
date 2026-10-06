(function () {
  var stored = localStorage.getItem("secretli-theme");
  var resolved = stored === "light" ? "light"
    : stored === "dark" ? "dark"
    : window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  if (resolved === "dark") document.documentElement.classList.add("dark");
  var meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.setAttribute("content", resolved === "dark" ? "#0a0a0b" : "#fafafa");
})();
