/** Hands a decrypted blob to the browser as a file download. */
export function saveBlob(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  // Revoking synchronously can cancel the download in some browsers.
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
}

/**
 * Saves several files one after another. Browsers throttle or block a burst of
 * downloads fired in a single tick.
 */
export async function saveFilesSequentially(files: Array<{ name: string; blob: Blob }>) {
  for (const [index, file] of files.entries()) {
    if (index > 0) {
      await new Promise((resolve) => window.setTimeout(resolve, 300));
    }
    saveBlob(file.blob, file.name);
  }
}
