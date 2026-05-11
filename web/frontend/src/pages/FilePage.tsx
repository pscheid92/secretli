import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { Link } from "react-router";
import { toast } from "sonner";
import ExpirationPicker from "../components/ExpirationPicker";
import FileUpload from "../components/FileUpload";
import SecretResult from "../components/SecretResult";
import Spinner from "../components/Spinner";
import Toggle from "../components/Toggle";
import { ApiError, createSecret } from "../lib/api";
import { createEncryptedBundle } from "../lib/bundle";
import { KeySet } from "../lib/encryption";
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

function ShareTabBar() {
  return (
    <div className="flex border-b border-zinc-200 dark:border-zinc-500/50 mb-6">
      <Link
        to="/share"
        className="px-1 pb-3 mr-6 text-sm text-zinc-600 dark:text-zinc-100 hover:text-zinc-900 dark:hover:text-white border-b-2 border-transparent transition-colors duration-150"
      >
        Text
      </Link>
      <div className="px-1 pb-3 text-sm font-medium text-zinc-900 dark:text-zinc-100 border-b-2 border-amber-400">
        File
      </div>
    </div>
  );
}

export default function FilePage() {
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
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

  async function onSubmit(data: FileFormData) {
    setLoading(true);

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
      toast.success(data.files.length > 1 ? "Files uploaded" : "File uploaded");
    } catch (err) {
      if (err instanceof ApiError) {
        toast.error(err.message);
      } else {
        toast.error("An unexpected error occurred. Please try again.");
      }
    } finally {
      setLoading(false);
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
          ← Share another file
        </button>
      </div>
    );
  }

  return (
    <div>
      <ShareTabBar />
      <form onSubmit={handleSubmit(onSubmit)} className="space-y-5">
        {/* File upload */}
        <div>
          <FileUpload
            onSelect={(selected) => {
              setValue("files", selected, { shouldValidate: true });
            }}
          />
          {errors.files && (
            <p className="mt-1.5 text-xs text-red-500 dark:text-red-400">{errors.files.message}</p>
          )}
        </div>

        {/* Expiration */}
        <div className="space-y-2">
          <span className="block text-xs tracking-widest uppercase text-zinc-600 dark:text-zinc-100">
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

        {/* Options */}
        <div className="rounded-lg border border-zinc-200 dark:border-zinc-500 bg-white dark:bg-zinc-900 divide-y divide-zinc-200 dark:divide-zinc-500/60">
          <div className="px-4 py-3">
            <Toggle
              checked={burnAfterRead}
              onChange={() => setValue("burnAfterRead", !burnAfterRead, { shouldValidate: true })}
              label="Burn after reading"
              description="Consumed when the recipient starts download"
            />
          </div>
          <div className="px-4 py-3">
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
            <div className="px-4 py-3">
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
                className="w-full rounded-md border border-zinc-200 dark:border-zinc-500 bg-zinc-50 dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 px-3 py-2.5 text-sm placeholder:text-zinc-500 dark:placeholder:text-zinc-500 focus:outline-none focus:border-amber-400 dark:focus:border-amber-400 focus:ring-1 focus:ring-amber-400/20 transition-colors duration-150"
              />
              {errors.password && (
                <p className="mt-1.5 text-xs text-red-500 dark:text-red-400">
                  {errors.password.message}
                </p>
              )}
            </div>
          )}
        </div>

        {/* Submit */}
        <button
          type="submit"
          disabled={loading || files.length === 0}
          className="flex w-full items-center justify-center gap-2 rounded-lg bg-amber-400 px-4 py-3 text-sm font-medium text-zinc-900 hover:bg-amber-300 focus:outline-none focus:ring-2 focus:ring-amber-400/50 disabled:opacity-40 disabled:cursor-not-allowed transition-all duration-150"
        >
          {loading && <Spinner size="sm" className="text-zinc-700" />}
          {loading
            ? "Encrypting & uploading..."
            : files.length > 1
              ? "Encrypt & Share Files"
              : "Encrypt & Share File"}
        </button>
      </form>
    </div>
  );
}
