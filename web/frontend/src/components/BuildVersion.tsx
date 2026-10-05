import { useEffect, useState } from "react";
import { getVersion } from "../lib/api";

const COMMIT = /^[0-9a-f]{40}$/;

/** Shows which build is running. Renders nothing if the version can't be read. */
export default function BuildVersion() {
  const [version, setVersion] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    getVersion()
      .then((v) => {
        if (!cancelled) setVersion(v);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  if (!version) return null;

  return (
    <p className="mt-2 text-xs text-zinc-500 dark:text-zinc-400">
      {COMMIT.test(version) ? (
        <a
          href={`https://github.com/pscheid92/secretli/commit/${version}`}
          target="_blank"
          rel="noreferrer"
          className="transition-colors duration-150 hover:text-amber-500 dark:hover:text-amber-400"
        >
          Build {version.slice(0, 7)}
        </a>
      ) : (
        `Build ${version}`
      )}
    </p>
  );
}
