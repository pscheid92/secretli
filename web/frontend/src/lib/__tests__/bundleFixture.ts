import {
  BUNDLE_FOOTER_LENGTH,
  type BundleFooter,
  type BundleManifest,
  type BundlePlan,
  buildBundleFooter,
  bundleManifestAad,
  bundleRecordAad,
  planBundle,
  sha256Hex,
} from "../bundle";
import type { KeySet } from "../encryption";

const textEncoder = new TextEncoder();

/**
 * Builds a complete encrypted bundle in memory. Production streams records
 * straight to the server instead (see multipartBundleUpload), so this exists
 * only to give tests a ready-made bundle to read back.
 */
export async function createEncryptedBundle(
  files: File[],
  keySet: KeySet,
  bundleName?: string,
): Promise<{ blob: Blob; manifest: BundleManifest; footer: BundleFooter }> {
  const plan = planBundle(files, bundleName);
  const { parts, manifest, encryptedManifest, footer } = await encryptBundlePlan(plan, keySet);

  return {
    blob: new Blob(
      [...parts, toArrayBuffer(encryptedManifest), toArrayBuffer(buildBundleFooter(footer))],
      {
        type: "application/octet-stream",
      },
    ),
    manifest,
    footer,
  };
}

async function encryptBundlePlan(
  plan: BundlePlan,
  keySet: KeySet,
): Promise<{
  parts: ArrayBuffer[];
  manifest: BundleManifest;
  encryptedManifest: Uint8Array;
  footer: BundleFooter;
}> {
  const parts: ArrayBuffer[] = [];

  for (const record of plan.records) {
    const file = plan.files[record.fileIndex];
    const plaintext = new Uint8Array(await file.slice(record.start, record.end).arrayBuffer());
    if (plaintext.length !== record.plaintextSize) {
      throw new Error("bundle file changed during encryption");
    }

    const encrypted = keySet.encryptBundlePart(
      plaintext,
      bundleRecordAad(record.fileIndex, record.chunkIndex, record.plaintextSize),
    );
    if (encrypted.length !== record.length) {
      throw new Error("bundle record size mismatch");
    }
    parts.push(toArrayBuffer(encrypted));
  }

  const manifest = plan.manifest;
  const encryptedManifest = keySet.encryptBundlePart(
    textEncoder.encode(JSON.stringify(manifest)),
    bundleManifestAad(),
  );
  if (encryptedManifest.length !== plan.encryptedManifestLength) {
    throw new Error("bundle manifest size mismatch");
  }

  const footer: BundleFooter = {
    version: 2,
    footerLength: BUNDLE_FOOTER_LENGTH,
    manifestLength: encryptedManifest.length,
    manifestSha256: await sha256Hex(encryptedManifest),
  };

  return { parts, manifest, encryptedManifest, footer };
}

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}
