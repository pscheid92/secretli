import { useState } from "react";
import { toast } from "sonner";
import SecretForm, { type SecretFormData } from "../components/SecretForm";
import SecretResult from "../components/SecretResult";
import { ApiError, createSecret } from "../lib/api";
import { KeySet } from "../lib/encryption";

interface ShareResult {
  url: string;
  expiresAt: string;
  burnAfterRead: boolean;
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

      const encrypted = await encryptKeySet.encrypt(data.text);
      const encoded = keySet.getEncoded();

      const response = await createSecret({
        public_id: encoded.publicID,
        retrieval_token: encoded.retrievalToken,
        deletion_token: encoded.deletionToken,
        nonce: encrypted.nonce,
        encrypted_data: encrypted.encrypted_data,
        expiration: data.expiration,
        burn_after_read: data.burnAfterRead,
        password_protected: hasPassword,
      });

      const shareUrl = `${window.location.origin}/s#${encoded.shareSecret}!${encoded.deletionToken}`;

      setResult({
        url: shareUrl,
        expiresAt: response.expires_at,
        burnAfterRead: data.burnAfterRead,
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

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold dark:text-white">Share a Secret</h1>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
          Your secret is encrypted in your browser before being sent to the server. Only the link
          holder can decrypt it.
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
            onClick={() => setResult(null)}
            className="text-sm text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
          >
            Share another secret
          </button>
        </div>
      ) : (
        <SecretForm onSubmit={handleSubmit} loading={loading} />
      )}
    </div>
  );
}
