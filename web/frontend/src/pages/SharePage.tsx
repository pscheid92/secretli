import { useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import SecretForm, { type SecretFormData } from "../components/SecretForm";
import SecretResult from "../components/SecretResult";
import { ApiError, createSecret } from "../lib/api";
import { KeySet } from "../lib/encryption";

interface ShareResult {
  url: string;
  expiresAt: string;
  burnAfterRead: boolean;
  deletionToken: string;
}

function ShareTabBar() {
  return (
    <div className="flex border-b border-zinc-200 dark:border-zinc-500/50 mb-6">
      <div className="px-1 pb-3 mr-6 text-sm font-medium text-zinc-900 dark:text-zinc-100 border-b-2 border-amber-400">
        Text
      </div>
      <Link
        to="/file"
        className="px-1 pb-3 text-sm text-zinc-600 dark:text-zinc-100 hover:text-zinc-900 dark:hover:text-white border-b-2 border-transparent transition-colors duration-150"
      >
        File
      </Link>
    </div>
  );
}

export default function SharePage() {
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<ShareResult | null>(null);

  async function handleSubmit(data: SecretFormData) {
    setLoading(true);

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

      const response = await createSecret(
        {
          public_id: encoded.publicID,
          retrieval_token: encoded.retrievalToken,
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
      toast.success("Secret created");
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
          onClick={() => setResult(null)}
          className="text-xs text-zinc-500 dark:text-zinc-100 hover:text-amber-500 dark:hover:text-amber-400 transition-colors duration-150"
        >
          ← Share another secret
        </button>
      </div>
    );
  }

  return (
    <div>
      <ShareTabBar />
      <SecretForm onSubmit={handleSubmit} loading={loading} />
    </div>
  );
}
