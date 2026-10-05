import { Link, Outlet, useLocation } from "react-router";
import { Toaster } from "sonner";
import { useLeaveWarningActive } from "../hooks/useLeaveWarning";
import { useTheme } from "../hooks/useTheme";
import BuildVersion from "./BuildVersion";
import IconButton from "./ui/IconButton";
import { LockIcon, MonitorIcon, MoonIcon, SunIcon } from "./ui/icons";
import { FOCUS } from "./ui/styles";

/** One column for header, content and footer: 640 px of content plus gutters. */
const COLUMN = "mx-auto w-full max-w-[43rem] px-5 sm:px-6";

function ThemeIcon({ theme }: { theme: string }) {
  if (theme === "dark") return <MoonIcon />;
  if (theme === "light") return <SunIcon />;
  return <MonitorIcon />;
}

function navLinkClass(active: boolean): string {
  return `inline-flex min-h-11 items-center rounded-[10px] px-3 text-sm font-medium transition-colors duration-150 ${FOCUS} ${
    active ? "text-ink" : "text-muted hover:text-ink"
  }`;
}

const FOOTER_LINK = `inline-flex min-h-11 items-center rounded-md transition-colors duration-150 hover:text-ink ${FOCUS}`;

export default function Layout() {
  const { theme, cycle } = useTheme();
  const location = useLocation();
  const reloadDocument = useLeaveWarningActive();

  const isShareActive = location.pathname === "/" || location.pathname === "/share";
  const isOpenActive = location.pathname === "/s" || location.pathname === "/c";

  return (
    <div className="flex min-h-screen flex-col bg-bg text-body text-ink">
      {/* Toasts follow the theme chosen here, not just the system's. */}
      <Toaster
        theme={theme}
        position="bottom-right"
        closeButton
        toastOptions={{ className: "font-sans" }}
      />
      <header className={`${COLUMN} flex items-center justify-between gap-4 pt-6`}>
        <Link
          to="/"
          reloadDocument={reloadDocument}
          className={`inline-flex min-h-11 items-center rounded-lg text-ink ${FOCUS}`}
        >
          <span className="inline-flex items-baseline gap-0.5 text-[19px] font-semibold tracking-[-0.035em]">
            secretli
            <span aria-hidden="true" className="h-1.5 w-1.5 rounded-full bg-accent" />
          </span>
        </Link>

        <nav aria-label="Main" className="flex items-center gap-0.5">
          <Link
            to="/share"
            reloadDocument={reloadDocument}
            aria-current={isShareActive ? "page" : undefined}
            className={navLinkClass(isShareActive)}
          >
            Share
          </Link>
          <Link
            to="/s"
            reloadDocument={reloadDocument}
            aria-current={isOpenActive ? "page" : undefined}
            className={navLinkClass(isOpenActive)}
          >
            Open
          </Link>
          <IconButton
            label={`Switch theme (currently ${theme})`}
            onClick={cycle}
            className="ml-1.5"
          >
            <ThemeIcon theme={theme} />
          </IconButton>
        </nav>
      </header>

      <main className={`${COLUMN} flex-1 pt-[clamp(2.5rem,7vw,6rem)] pb-18`}>
        <Outlet />
      </main>

      <footer
        className={`${COLUMN} flex flex-wrap items-center justify-between gap-x-5 pt-5 pb-7 text-[13px] text-faint`}
      >
        <span className="inline-flex min-h-11 items-center gap-2">
          <LockIcon />
          End-to-end encrypted in your browser
        </span>
        <span className="flex items-center gap-4">
          <a
            href="https://github.com/pscheid92/secretli"
            target="_blank"
            rel="noreferrer"
            className={FOOTER_LINK}
          >
            Source
          </a>
          <BuildVersion />
        </span>
      </footer>
    </div>
  );
}
