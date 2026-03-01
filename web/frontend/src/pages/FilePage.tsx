import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { toast } from "sonner";
import ExpirationPicker from "../components/ExpirationPicker";
import FileUpload from "../components/FileUpload";
import SecretResult from "../components/SecretResult";
import Spinner from "../components/Spinner";
import { ApiError, uploadFile } from "../lib/api";
import { KeySet } from "../lib/encryption";

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

  async function onSubmit(data: FileFormData) {
    setLoading(true);

    try {
      let fileToUpload: File;
      if (data.files.length > 1) {
        const JSZip = (await import("jszip")).default;
        const zip = new JSZip();
        for (const f of data.files) {
          zip.file(f.name, f);
        }
        const blob = await zip.generateAsync({ type: "blob" });
        fileToUpload = new File([blob], "multiple.zip", { type: "application/zip" });
      } else {
        fileToUpload = data.files[0];
      }

      const keySet = await KeySet.generateRandom();
      const hasPassword = data.password.length > 0;

      let encryptKeySet = keySet;
      if (hasPassword) {
        const encoded = keySet.getEncoded();
        encryptKeySet = await KeySet.fromShareSecret(encoded.shareSecret, data.password);
      }

      const fileBytes = new Uint8Array(await fileToUpload.arrayBuffer());
      const { nonce, encryptedBlob } = await encryptKeySet.encryptFile(fileBytes);
      const encryptedFilename = await encryptKeySet.encryptFilename(fileToUpload.name);

      const encoded = keySet.getEncoded();

      const response = await uploadFile(
        {
          public_id: encoded.publicID,
          retrieval_token: encoded.retrievalToken,
          deletion_token: encoded.deletionToken,
          nonce,
          expiration: data.expiration,
          burn_after_read: data.burnAfterRead,
          password_protected: hasPassword,
          encrypted_filename: encryptedFilename,
        },
        encryptedBlob,
      );

      const shareUrl = `${window.location.origin}/s#${encoded.shareSecret}!${encoded.deletionToken}`;

      setResult({
        url: shareUrl,
        expiresAt: response.expires_at,
        burnAfterRead: data.burnAfterRead,
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

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold dark:text-white">Share a File</h1>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          Your file is encrypted in your browser before being uploaded. Only the link holder can
          decrypt it.
        </p>
      </div>

      {result ? (
        <div className="space-y-4">
          <SecretResult
            url={result.url}
            expiresAt={result.expiresAt}
            burnAfterRead={result.burnAfterRead}
          />
          <button
            type="button"
            onClick={() => {
              setResult(null);
              reset();
            }}
            className="text-sm font-medium text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 transition-colors duration-150"
          >
            Share another file
          </button>
        </div>
      ) : (
        <div className="rounded-xl border border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900 p-5 shadow-sm">
          <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
            <div>
              <FileUpload
                onSelect={(selected) => {
                  setValue("files", selected, { shouldValidate: true });
                }}
              />
              {errors.files && (
                <p className="mt-1 text-sm text-red-600 dark:text-red-400">
                  {errors.files.message}
                </p>
              )}
            </div>

            <div className="flex flex-wrap items-center gap-4">
              <div>
                <label
                  htmlFor="expiration"
                  className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
                >
                  Expires in
                </label>
                <Controller
                  name="expiration"
                  control={control}
                  render={({ field }) => (
                    <ExpirationPicker value={field.value} onChange={field.onChange} />
                  )}
                />
              </div>

              <label className="flex items-center gap-2 pt-5 text-sm text-gray-700 dark:text-gray-300 cursor-pointer">
                <input
                  type="checkbox"
                  {...register("burnAfterRead")}
                  className="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                />
                Burn after reading
              </label>
            </div>

            <div>
              <button
                type="button"
                onClick={() => setShowPassword(!showPassword)}
                className="text-sm font-medium text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300 transition-colors duration-150"
              >
                {showPassword ? "Remove password protection" : "Add password protection"}
              </button>
              {showPassword && (
                <>
                  <input
                    type="password"
                    {...register("password", {
                      validate: (v) => !showPassword || v.length > 0 || "Password is required",
                    })}
                    placeholder="Enter a password..."
                    className="mt-2 w-full rounded-lg border border-gray-300 dark:border-gray-600 bg-gray-50 dark:bg-gray-800 dark:text-white px-3.5 py-2.5 text-sm placeholder:text-gray-400 focus:bg-white dark:focus:bg-gray-800 focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20 transition-colors duration-150"
                  />
                  {errors.password && (
                    <p className="mt-1 text-sm text-red-600 dark:text-red-400">
                      {errors.password.message}
                    </p>
                  )}
                </>
              )}
            </div>

            <button
              type="submit"
              disabled={loading || files.length === 0}
              className="flex w-full items-center justify-center gap-2 rounded-lg bg-blue-600 px-4 py-2.5 font-semibold text-white shadow-md hover:bg-blue-700 hover:shadow-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-150"
            >
              {loading && <Spinner size="sm" />}
              {loading
                ? "Encrypting & uploading..."
                : files.length > 1
                  ? "Share Files"
                  : "Share File"}
            </button>
          </form>
        </div>
      )}
    </div>
  );
}
