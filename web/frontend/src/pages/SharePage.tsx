import { useState } from "react";
import { toast } from "sonner";
import SecretForm, { type SecretFormData } from "../components/SecretForm";
import SecretResult from "../components/SecretResult";
import ShareModeTabs from "../components/ShareModeTabs";
import { ApiError, createSecret } from "../lib/api";
import { KeySet } from "../lib/encryption";

interface ShareResult {
  url: string;
  expiresAt: string;
  burnAfterRead: boolean;
  deletionToken: string;
}

export default function SharePage() {
  const [loading, setLoading] = useState(false);
  const [stage, setStage] = useState<"idle" | "encrypting" | "uploading">("idle");
  const [result, setResult] = useState<ShareResult | null>(null);

  async function handleSubmit(data: SecretFormData) {
    setLoading(true);
    setStage("encrypting");

    try {
      const keySet = await KeySet.generateRandom();
      const hasPassword = data.password.length > 0;

      let encryptKeySet = keySet;
      if (hasPassword) {
        const encoded = keySet.getEncoded();
        encryptKeySet = await KeySet.fromShareSecret(encoded.shareSecret, data.password);
      }

      const textBytes = new TextEncoder().encode(data.text);
      const blob = await encryptKeySet.encryptBlob(textBytes);
      const encryptedMeta = await keySet.encryptMeta({
        type: "text",
        password_protected: hasPassword,
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
          onClick={() => setResult(null)}
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
          <ShareModeTabs active="text" />
        </div>
      </div>
      <SecretForm onSubmit={handleSubmit} loading={loading} stage={stage} />
    </div>
  );
}
