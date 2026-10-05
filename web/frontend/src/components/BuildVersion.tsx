import { useEffect, useState } from "react";
import { getVersion } from "../lib/api";
import { FOCUS } from "./ui/styles";

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
    <span className="inline-flex min-h-11 items-center font-mono text-xs text-faint">
      {COMMIT.test(version) ? (
        <a
          href={`https://github.com/pscheid92/secretli/commit/${version}`}
          target="_blank"
          rel="noreferrer"
          className={`rounded-md transition-colors duration-150 hover:text-ink ${FOCUS}`}
        >
          Build {version.slice(0, 7)}
        </a>
      ) : (
        `Build ${version}`
      )}
    </span>
  );
}
