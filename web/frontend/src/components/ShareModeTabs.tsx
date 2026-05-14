import { Link } from "react-router";

export default function ShareModeTabs({ active }: { active: "text" | "files" }) {
  const itemClass = (mode: "text" | "files") =>
    `flex-1 rounded-md px-3 py-2 text-center text-sm font-medium transition-colors duration-150 ${
      active === mode
        ? "bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-950"
        : "text-zinc-600 hover:text-zinc-900 dark:text-zinc-300 dark:hover:text-white"
    }`;

  return (
    <div className="grid grid-cols-2 gap-1 rounded-lg border border-zinc-200 bg-zinc-100 p-1 dark:border-zinc-600 dark:bg-zinc-900">
      <Link
        to="/share"
        className={itemClass("text")}
        aria-current={active === "text" ? "page" : undefined}
      >
        Text
      </Link>
      <Link
        to="/file"
        className={itemClass("files")}
        aria-current={active === "files" ? "page" : undefined}
      >
        Files
      </Link>
    </div>
  );
}
