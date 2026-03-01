(function () {
  var stored = localStorage.getItem("secretli-theme");
  var resolved = stored === "light" ? "light"
    : stored === "dark" ? "dark"
    : window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  if (resolved === "dark") document.documentElement.classList.add("dark");
})();
