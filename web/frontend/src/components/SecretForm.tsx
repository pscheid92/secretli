import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { formatExpiration } from "../lib/expiration";
import { formatSize } from "../lib/format";
import ExpirationPicker from "./ExpirationPicker";
import Spinner from "./Spinner";
import Toggle from "./Toggle";
import TransferStatus, { type TransferStep } from "./TransferStatus";

export interface SecretFormData {
  text: string;
  expiration: string;
  burnAfterRead: boolean;
  password: string;
}

interface SecretFormProps {
  onSubmit: (data: SecretFormData) => void;
  loading: boolean;
  stage: "idle" | "encrypting" | "uploading";
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
  const rows = ["Encrypt locally", "Upload encrypted data", "Issue links"];
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

export default function SecretForm({ onSubmit, loading, stage }: SecretFormProps) {
  const [showPassword, setShowPassword] = useState(false);

  const {
    register,
    handleSubmit,
    control,
    setValue,
    watch,
    formState: { errors },
  } = useForm<SecretFormData>({
    defaultValues: {
      text: "",
      expiration: "1d",
      burnAfterRead: false,
      password: "",
    },
  });

  const burnAfterRead = watch("burnAfterRead");
  const text = watch("text");
  const expiration = watch("expiration");
  const password = watch("password");
  const encodedSize = new TextEncoder().encode(text).length;
  const steps: TransferStep[] = [
    {
      label: "Encrypting",
      state: stage === "idle" ? "pending" : stage === "encrypting" ? "active" : "done",
    },
    { label: "Uploading", state: stage === "uploading" ? "active" : "pending" },
    { label: "Creating link", state: "pending" },
  ];

  return (
    <form
      onSubmit={handleSubmit(onSubmit)}
      className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px] lg:items-start"
    >
      <div className="space-y-5">
        <section className="rounded-lg border border-zinc-200 bg-white dark:border-zinc-700 dark:bg-zinc-900">
          <div className="border-b border-zinc-200 px-4 py-3 dark:border-zinc-700">
            <div>
              <h2 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">Text</h2>
              <p className="mt-0.5 text-xs text-zinc-500 dark:text-zinc-400">
                {encodedSize > 0 ? formatSize(encodedSize) : "No text"}
              </p>
            </div>
          </div>
          <div className="p-4">
            <textarea
              id="secret-text"
              {...register("text", { required: "Text is required" })}
              placeholder="Type or paste text here..."
              rows={10}
              data-gramm="false"
              data-gramm_editor="false"
              data-enable-grammarly="false"
              data-1p-ignore
              className="block h-64 w-full resize-none rounded-md border border-zinc-200 bg-zinc-50 px-4 py-3 text-sm text-zinc-900 placeholder:text-zinc-500 transition-colors duration-150 focus:border-amber-400 focus:outline-none focus:ring-1 focus:ring-amber-400/20 dark:border-zinc-700 dark:bg-zinc-950 dark:text-zinc-100 dark:placeholder:text-zinc-500 dark:focus:border-amber-400"
            />
            {errors.text && (
              <p className="mt-2 text-xs text-red-500 dark:text-red-400">{errors.text.message}</p>
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
                  description="Consumed when the recipient starts reveal"
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
                    autoFocus
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
            <SummaryMetric label="Source" value="Text" />
            <SummaryMetric label="Size" value={encodedSize > 0 ? formatSize(encodedSize) : "-"} />
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
          disabled={loading || !text.trim()}
          className="flex w-full items-center justify-center gap-2 rounded-lg bg-amber-400 px-4 py-3 text-sm font-semibold text-zinc-950 transition-all duration-150 hover:bg-amber-300 focus:outline-none focus:ring-2 focus:ring-amber-400/50 disabled:cursor-not-allowed disabled:opacity-40"
        >
          {loading && <Spinner size="sm" className="text-zinc-700" />}
          {loading ? "Working..." : "Create Secure Link"}
        </button>
      </aside>
    </form>
  );
}
