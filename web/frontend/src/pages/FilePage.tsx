import { useEffect, useRef, useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { toast } from "sonner";
import ExpirationPicker from "../components/ExpirationPicker";
import FileUpload from "../components/FileUpload";
import SecretResult from "../components/SecretResult";
import ShareModeTabs from "../components/ShareModeTabs";
import Spinner from "../components/Spinner";
import Toggle from "../components/Toggle";
import TransferStatus, {
  type TransferProgress,
  type TransferStep,
} from "../components/TransferStatus";
import { ApiError } from "../lib/api";
import { KeySet } from "../lib/encryption";
import { formatExpiration } from "../lib/expiration";
import { formatSize } from "../lib/format";
import { UploadCancelledError, uploadMultipartBundle } from "../lib/multipartBundleUpload";
import {
  fitsBundleManifestLimit,
  fitsBundleUploadLimit,
  MAX_UPLOAD_LABEL,
} from "../lib/uploadLimits";

interface FileFormData {
  files: File[];
  expiration: string;
  burnAfterRead: boolean;
  password: string;
}

interface FileResult {
  url: string;
  expiresAt: string;
  burnAfterRead: boolean;
  deletionToken: string;
}

function SummaryMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="border-t border-zinc-200 py-3 first:border-t-0 dark:border-zinc-700">
      <div className="text-xs text-zinc-500 dark:text-zinc-400">{label}</div>
      <div className="mt-1 text-sm font-medium text-zinc-900 dark:text-zinc-100">{value}</div>
    </div>
  );
}

function TransferPreview() {
  const rows = ["Encrypt bundle", "Upload encrypted data", "Issue links"];
  return (
    <section className="rounded-lg border border-zinc-200 bg-white px-4 py-4 dark:border-zinc-700 dark:bg-zinc-900">
      <h2 className="text-xs font-semibold uppercase tracking-widest text-zinc-500 dark:text-zinc-400">
        Transfer
      </h2>
      <div className="mt-3 space-y-2">
        {rows.map((row, index) => (
          <div
            key={row}
            className="flex items-center gap-2 text-sm text-zinc-700 dark:text-zinc-200"
          >
            <span className="flex h-5 w-5 items-center justify-center rounded-full border border-zinc-200 text-[10px] font-semibold text-zinc-500 dark:border-zinc-700 dark:text-zinc-400">
              {index + 1}
            </span>
            <span>{row}</span>
          </div>
        ))}
      </div>
    </section>
  );
}

export default function FilePage() {
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [stage, setStage] = useState<"idle" | "encrypting" | "uploading">("idle");
  const [progress, setProgress] = useState<TransferProgress | null>(null);
  const [result, setResult] = useState<FileResult | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  // A navigation away from an in-flight upload silently discards it.
  useEffect(() => {
    if (!loading) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [loading]);

  const {
    register,
    handleSubmit,
    control,
    setValue,
    watch,
    formState: { errors },
    reset,
  } = useForm<FileFormData>({
    defaultValues: {
      files: [],
      expiration: "1d",
      burnAfterRead: false,
      password: "",
    },
  });

  const files = watch("files");
  const burnAfterRead = watch("burnAfterRead");
  const expiration = watch("expiration");
  const password = watch("password");
  const totalSize = files.reduce((sum, file) => sum + file.size, 0);
  const steps: TransferStep[] = [
    {
      label: "Encrypting",
      state: stage === "idle" ? "pending" : stage === "encrypting" ? "active" : "done",
    },
    { label: "Uploading", state: stage === "uploading" ? "active" : "pending" },
    { label: "Finalize", state: "pending" },
  ];

  function cancelUpload() {
    abortRef.current?.abort();
  }

  async function onSubmit(data: FileFormData) {
    setLoading(true);
    setStage("encrypting");
    setProgress(null);
    const controller = new AbortController();
    abortRef.current = controller;

    try {
      if (data.files.length === 0) {
        toast.error("Select at least one file.");
        return;
      }

      if (!fitsBundleUploadLimit(data.files.map((file) => file.size))) {
        toast.error(`Selected files exceed the ${MAX_UPLOAD_LABEL} upload limit.`);
        return;
      }
      if (!fitsBundleManifestLimit(data.files)) {
        toast.error("Too many files for one share. Zip them first or split the share.");
        return;
      }

      const keySet = await KeySet.generateRandom();
      const hasPassword = data.password.length > 0;

      let encryptKeySet = keySet;
      if (hasPassword) {
        const encoded = keySet.getEncoded();
        encryptKeySet = await KeySet.fromShareSecret(encoded.shareSecret, data.password);
      }

      // Files are always encrypted record by record and streamed as multipart
      // parts, whatever their size, so there is a single upload path.
      setStage("uploading");
      const response = await uploadMultipartBundle({
        files: data.files,
        baseKeySet: keySet,
        bundleKeySet: encryptKeySet,
        passwordProtected: hasPassword,
        expiration: data.expiration,
        burnAfterRead: data.burnAfterRead,
        signal: controller.signal,
        onProgress: ({ uploadedBytes, totalBytes }) => {
          setProgress({
            fraction: totalBytes > 0 ? uploadedBytes / totalBytes : 0,
            label: `${formatSize(uploadedBytes)} / ${formatSize(totalBytes)}`,
          });
        },
      });

      setResult({
        url: `${window.location.origin}/s#${response.encoded.shareSecret}`,
        expiresAt: response.expires_at,
        burnAfterRead: data.burnAfterRead,
        deletionToken: response.deletionToken,
      });
      toast.success("Share created");
    } catch (err) {
      if (err instanceof UploadCancelledError) {
        toast.info("Upload cancelled.");
      } else if (err instanceof ApiError) {
        toast.error(err.message);
      } else if (err instanceof Error && err.message === "bundle manifest is too large") {
        toast.error("Too many files for one share. Zip them first or split the share.");
      } else {
        toast.error("An unexpected error occurred. Please try again.");
      }
    } finally {
      abortRef.current = null;
      setLoading(false);
      setStage("idle");
      setProgress(null);
    }
  }

  if (result) {
    return (
      <div className="space-y-5">
        <SecretResult
          url={result.url}
          expiresAt={result.expiresAt}
          burnAfterRead={result.burnAfterRead}
          deletionToken={result.deletionToken}
        />
        <button
          type="button"
          onClick={() => {
            setResult(null);
            reset();
          }}
          className="text-xs text-zinc-500 dark:text-zinc-100 hover:text-amber-500 dark:hover:text-amber-400 transition-colors duration-150"
        >
          ← Create another share
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 border-b border-zinc-200 pb-5 dark:border-zinc-800 md:flex-row md:items-end md:justify-between">
        <h1 className="font-display text-2xl font-semibold text-zinc-800 dark:text-zinc-100">
          Create Share
        </h1>
        <div className="w-full md:w-72">
          <ShareModeTabs active="files" />
        </div>
      </div>
      <form
        onSubmit={handleSubmit(onSubmit)}
        className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px] lg:items-start"
      >
        <div className="space-y-5">
          <section className="rounded-lg border border-zinc-200 bg-white dark:border-zinc-700 dark:bg-zinc-900">
            <div className="border-b border-zinc-200 px-4 py-3 dark:border-zinc-700">
              <div>
                <h2 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">Files</h2>
                <p className="mt-0.5 text-xs text-zinc-500 dark:text-zinc-400">
                  {files.length > 0
                    ? `${files.length} ${files.length === 1 ? "file" : "files"} · ${formatSize(totalSize)}`
                    : "No files"}
                </p>
              </div>
            </div>
            <div className="p-4">
              <FileUpload
                onSelect={(selected) => {
                  setValue("files", selected, { shouldValidate: true });
                }}
              />
              {errors.files && (
                <p className="mt-2 text-xs text-red-500 dark:text-red-400">
                  {errors.files.message}
                </p>
              )}
            </div>
          </section>

          <section className="rounded-lg border border-zinc-200 bg-white dark:border-zinc-700 dark:bg-zinc-900">
            <div className="border-b border-zinc-200 px-4 py-3 dark:border-zinc-700">
              <h2 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">Rules</h2>
            </div>
            <div className="space-y-5 p-4">
              <div className="space-y-2">
                <span className="block text-xs font-semibold uppercase tracking-widest text-zinc-500 dark:text-zinc-400">
                  Expires in
                </span>
                <Controller
                  name="expiration"
                  control={control}
                  render={({ field }) => (
                    <ExpirationPicker value={field.value} onChange={field.onChange} />
                  )}
                />
              </div>

              <div className="divide-y divide-zinc-200 rounded-md border border-zinc-200 dark:divide-zinc-700 dark:border-zinc-700">
                <div className="px-3 py-3">
                  <Toggle
                    checked={burnAfterRead}
                    onChange={() =>
                      setValue("burnAfterRead", !burnAfterRead, { shouldValidate: true })
                    }
                    label="Burn after reading"
                    description="Consumed when the recipient starts download"
                  />
                </div>
                <div className="px-3 py-3">
                  <Toggle
                    checked={showPassword}
                    onChange={() => {
                      setShowPassword(!showPassword);
                      if (showPassword) setValue("password", "");
                    }}
                    label="Password protection"
                    description="Require a password to decrypt"
                  />
                </div>
                {showPassword && (
                  <div className="px-3 py-3">
                    <input
                      type="password"
                      {...register("password", {
                        validate: (v) => !showPassword || v.length > 0 || "Password is required",
                      })}
                      placeholder="Enter a password..."
                      autoComplete="off"
                      data-gramm="false"
                      data-gramm_editor="false"
                      data-enable-grammarly="false"
                      data-1p-ignore
                      className="w-full rounded-md border border-zinc-200 bg-zinc-50 px-3 py-2.5 text-sm text-zinc-900 placeholder:text-zinc-500 transition-colors duration-150 focus:border-amber-400 focus:outline-none focus:ring-1 focus:ring-amber-400/20 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100 dark:placeholder:text-zinc-500 dark:focus:border-amber-400"
                    />
                    {errors.password && (
                      <p className="mt-2 text-xs text-red-500 dark:text-red-400">
                        {errors.password.message}
                      </p>
                    )}
                  </div>
                )}
              </div>
            </div>
          </section>
        </div>

        <aside className="space-y-4 lg:sticky lg:top-24">
          <section className="rounded-lg border border-zinc-200 bg-white px-4 py-4 dark:border-zinc-700 dark:bg-zinc-900">
            <h2 className="text-xs font-semibold uppercase tracking-widest text-zinc-500 dark:text-zinc-400">
              Share summary
            </h2>
            <div className="mt-2">
              <SummaryMetric label="Files" value={files.length > 0 ? String(files.length) : "-"} />
              <SummaryMetric label="Size" value={totalSize > 0 ? formatSize(totalSize) : "-"} />
              <SummaryMetric label="Expires" value={formatExpiration(expiration)} />
              <SummaryMetric
                label="Protection"
                value={password ? "Password" : burnAfterRead ? "Burn" : "Standard"}
              />
            </div>
          </section>

          {loading ? (
            <TransferStatus
              title="Creating secure link"
              steps={steps}
              progress={progress ?? undefined}
              onCancel={stage === "uploading" && progress ? cancelUpload : undefined}
              cancelLabel="Cancel upload"
            />
          ) : (
            <TransferPreview />
          )}

          <button
            type="submit"
            disabled={loading || files.length === 0}
            className="flex w-full items-center justify-center gap-2 rounded-lg bg-amber-400 px-4 py-3 text-sm font-semibold text-zinc-950 transition-all duration-150 hover:bg-amber-300 focus:outline-none focus:ring-2 focus:ring-amber-400/50 disabled:cursor-not-allowed disabled:opacity-40"
          >
            {loading && <Spinner size="sm" className="text-zinc-700" />}
            {loading ? "Working..." : "Create Secure Link"}
          </button>
        </aside>
      </form>
    </div>
  );
}
