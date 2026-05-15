import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { toast } from "sonner";
import ExpirationPicker from "../components/ExpirationPicker";
import FileUpload from "../components/FileUpload";
import SecretResult from "../components/SecretResult";
import ShareModeTabs from "../components/ShareModeTabs";
import Spinner from "../components/Spinner";
import Toggle from "../components/Toggle";
import TransferStatus, { type TransferStep } from "../components/TransferStatus";
import { ApiError, createSecret } from "../lib/api";
import { createEncryptedBundle, estimateBundleEncryptedSize } from "../lib/bundle";
import { KeySet } from "../lib/encryption";
import { formatExpiration } from "../lib/expiration";
import { formatSize } from "../lib/format";
import {
  LARGE_BUNDLE_MULTIPART_THRESHOLD_BYTES,
  uploadMultipartBundle,
} from "../lib/multipartBundleUpload";
import {
  fitsBundleUploadLimit,
  MAX_ENCRYPTED_UPLOAD_BYTES,
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
  const [result, setResult] = useState<FileResult | null>(null);

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

  async function onSubmit(data: FileFormData) {
    setLoading(true);
    setStage("encrypting");

    try {
      if (data.files.length === 0) {
        toast.error("Select at least one file.");
        return;
      }

      if (!fitsBundleUploadLimit(data.files.map((file) => file.size))) {
        toast.error(`Selected files exceed the ${MAX_UPLOAD_LABEL} upload limit.`);
        return;
      }

      const keySet = await KeySet.generateRandom();
      const hasPassword = data.password.length > 0;

      let encryptKeySet = keySet;
      if (hasPassword) {
        const encoded = keySet.getEncoded();
        encryptKeySet = await KeySet.fromShareSecret(encoded.shareSecret, data.password);
      }

      const estimatedBundleSize = estimateBundleEncryptedSize(data.files.map((file) => file.size));
      if (estimatedBundleSize >= LARGE_BUNDLE_MULTIPART_THRESHOLD_BYTES) {
        setStage("uploading");
        const response = await uploadMultipartBundle({
          files: data.files,
          baseKeySet: keySet,
          bundleKeySet: encryptKeySet,
          password: hasPassword ? data.password : undefined,
          passwordProtected: hasPassword,
          expiration: data.expiration,
          burnAfterRead: data.burnAfterRead,
        });

        setResult({
          url: `${window.location.origin}/s#${response.encoded.shareSecret}`,
          expiresAt: response.expires_at,
          burnAfterRead: data.burnAfterRead,
          deletionToken: response.deletionToken,
        });
        toast.success("Share created");
        return;
      }

      const { blob, manifest } = await createEncryptedBundle(data.files, encryptKeySet);
      if (blob.size > MAX_ENCRYPTED_UPLOAD_BYTES) {
        toast.error(`Encrypted file exceeds the ${MAX_UPLOAD_LABEL} upload limit.`);
        return;
      }

      const encryptedMeta = await keySet.encryptMeta({
        type: "bundle",
        password_protected: hasPassword,
        bundle_name: manifest.bundleName,
      });

      const encoded = keySet.getEncoded();
      setStage("uploading");

      const response = await createSecret(
        {
          public_id: encoded.publicID,
          metadata_token: encoded.metadataToken,
          blob_token: encryptKeySet.getEncoded().blobToken,
          deletion_token: encoded.deletionToken,
          encrypted_meta: encryptedMeta,
          expiration: data.expiration,
          burn_after_read: data.burnAfterRead,
        },
        blob,
      );

      setResult({
        url: `${window.location.origin}/s#${encoded.shareSecret}`,
        expiresAt: response.expires_at,
        burnAfterRead: data.burnAfterRead,
        deletionToken: encoded.deletionToken,
      });
      toast.success("Share created");
    } catch (err) {
      if (err instanceof ApiError) {
        toast.error(err.message);
      } else {
        toast.error("An unexpected error occurred. Please try again.");
      }
    } finally {
      setLoading(false);
      setStage("idle");
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
            <TransferStatus title="Creating secure link" steps={steps} />
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
