import { Link } from "react-router";

export default function LandingPage() {
  return (
    <div className="flex flex-col items-center justify-center py-16 sm:py-24 text-center">
      {/* Badge */}
      <div className="mb-8 inline-flex items-center gap-2 rounded-full border border-zinc-200 dark:border-zinc-500/50 px-3.5 py-1.5 text-xs tracking-widest uppercase text-zinc-600 dark:text-zinc-100">
        <span className="w-1.5 h-1.5 rounded-full bg-amber-400 animate-pulse" />
        End-to-end encrypted
      </div>

      {/* Hero */}
      <h1 className="font-display text-5xl sm:text-6xl font-medium leading-[1.1] tracking-tight text-zinc-800 dark:text-zinc-100">
        Share Secrets.
        <br />
        <span className="text-amber-500 dark:text-amber-400">Leave No Trace.</span>
      </h1>

      <p className="mt-6 text-sm text-zinc-600 dark:text-zinc-100 max-w-xs leading-relaxed">
        Encrypt anything in your browser. Generate a link. It disappears automatically.
      </p>

      {/* CTAs */}
      <div className="mt-10 flex flex-col sm:flex-row gap-3 w-full sm:w-auto">
        <Link
          to="/share"
          className="inline-flex items-center justify-center rounded-lg bg-amber-400 px-8 py-3 text-sm font-medium text-zinc-900 hover:bg-amber-300 focus:outline-none focus:ring-2 focus:ring-amber-400/50 transition-colors duration-150"
        >
          Share a Secret
        </Link>
        <Link
          to="/s"
          className="inline-flex items-center justify-center rounded-lg border border-zinc-200 dark:border-zinc-500/50 px-8 py-3 text-sm text-zinc-600 dark:text-zinc-100 hover:border-zinc-300 dark:hover:border-zinc-600 hover:text-zinc-900 dark:hover:text-white focus:outline-none focus:ring-2 focus:ring-zinc-400/20 transition-colors duration-150"
        >
          Retrieve a Secret
        </Link>
      </div>

      {/* Feature list */}
      <div className="mt-16 flex flex-wrap justify-center gap-x-8 gap-y-2">
        {[
          "AES-256-GCM encryption",
          "Auto-expires",
          "Burn after reading",
          "No account required",
          "Password protection",
        ].map((feature) => (
          <span key={feature} className="inline-flex items-center gap-1.5 text-xs text-zinc-500 dark:text-zinc-100">
            <span className="w-1 h-1 rounded-full bg-zinc-300 dark:bg-zinc-700" />
            {feature}
          </span>
        ))}
      </div>
    </div>
  );
}
