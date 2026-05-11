import { createHash } from "node:crypto";
import { createReadStream, createWriteStream } from "node:fs";
import { mkdir, stat } from "node:fs/promises";
import { dirname } from "node:path";
import { expect, type Page, test } from "@playwright/test";

const DEFAULT_SIZE_MIB = 99;
const TEST_TIMEOUT_MS = 10 * 60 * 1000;
const MIB = 1024 * 1024;

interface HeapSample {
  readonly label: string;
  readonly cdpUsedBytes?: number;
  readonly cdpTotalBytes?: number;
  readonly performanceUsedBytes?: number;
}

interface Report {
  readonly fileSizeBytes: number;
  readonly fileSizeMiB: number;
  readonly uploadMs: number;
  readonly revealMs: number;
  readonly downloadMs: number;
  readonly retrieveTotalMs: number;
  readonly totalMs: number;
  readonly rangeRequestCount: number;
  readonly expectedSha256: string;
  readonly downloadedSha256: string;
  readonly heapSamples: HeapSample[];
}

test.describe("Large-file performance", () => {
  test.skip(process.env.LARGE_E2E !== "1", "set LARGE_E2E=1 to run large-file tests");
  test.setTimeout(TEST_TIMEOUT_MS);

  test("uploads, retrieves, downloads, and verifies a near-limit bundle", async ({
    page,
    browserName,
  }, testInfo) => {
    test.skip(browserName !== "chromium", "heap sampling currently uses Chromium CDP");

    const sizeMiB = parseSizeMiB();
    const sizeBytes = sizeMiB * MIB;
    const filename = `secretli-large-${sizeMiB}MiB.bin`;
    const sourcePath = testInfo.outputPath("source", filename);
    const expectedSha256 = await writePatternedFile(sourcePath, sizeBytes);
    const heapSamples: HeapSample[] = [];
    const sampleHeap = heapSampler(page, heapSamples);
    let rangeRequestCount = 0;
    page.on("request", (request) => {
      if (request.method() !== "GET") return;
      const url = new URL(request.url());
      if (url.pathname.endsWith("/blob")) {
        rangeRequestCount++;
      }
    });

    const totalStartedAt = performance.now();
    await sampleHeap("initial");

    await page.goto("/file");
    await page.setInputFiles('input[type="file"]', sourcePath);
    await expect(page.getByText(`${sizeMiB.toFixed(1)} MB / 100 MB`)).toBeVisible({
      timeout: 10000,
    });

    const uploadStartedAt = performance.now();
    await page.click('button[type="submit"]');
    await expect(page.getByRole("main").getByText("Secret created")).toBeVisible({
      timeout: TEST_TIMEOUT_MS,
    });
    const uploadMs = performance.now() - uploadStartedAt;
    await sampleHeap("after upload");

    const shareUrl = await page.locator("input[readonly]").first().inputValue();
    await page.goto(shareUrl);
    await expect(page.locator("h1")).toHaveText("Secret Ready", { timeout: 10000 });

    const outputPath = testInfo.outputPath("downloaded", filename);
    const retrieveStartedAt = performance.now();
    const { revealMs, downloadMs } = await retrieveBundle(page, outputPath, filename, sampleHeap);
    const retrieveTotalMs = performance.now() - retrieveStartedAt;
    await sampleHeap("after download");

    const downloadedStats = await stat(outputPath);
    expect(downloadedStats.size).toBe(sizeBytes);

    const downloadedSha256 = await sha256File(outputPath);
    expect(downloadedSha256).toBe(expectedSha256);

    const totalMs = performance.now() - totalStartedAt;
    const report: Report = {
      fileSizeBytes: sizeBytes,
      fileSizeMiB: sizeMiB,
      uploadMs,
      revealMs,
      downloadMs,
      retrieveTotalMs,
      totalMs,
      rangeRequestCount,
      expectedSha256,
      downloadedSha256,
      heapSamples,
    };

    enforceOptionalThresholds(report);
    printReport(report);
  });
});

async function retrieveBundle(
  page: Page,
  outputPath: string,
  filename: string,
  sampleHeap: (label: string) => Promise<void>,
): Promise<{ revealMs: number; downloadMs: number }> {
  const revealStartedAt = performance.now();
  await page.getByRole("button", { name: /Download/ }).click();
  await expect(page.locator("h1")).toHaveText("File Ready", { timeout: TEST_TIMEOUT_MS });
  const revealMs = performance.now() - revealStartedAt;
  await sampleHeap("after manifest");

  const downloadStartedAt = performance.now();
  const downloadPromise = page.waitForEvent("download", { timeout: TEST_TIMEOUT_MS });
  await page.getByRole("button", { name: "Download File" }).click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe(filename);
  await download.saveAs(outputPath);
  const downloadMs = performance.now() - downloadStartedAt;

  return { revealMs, downloadMs };
}

function parseSizeMiB(): number {
  const value = Number.parseInt(process.env.LARGE_E2E_SIZE_MB ?? `${DEFAULT_SIZE_MIB}`, 10);
  if (!Number.isSafeInteger(value) || value <= 0) {
    throw new Error("LARGE_E2E_SIZE_MB must be a positive integer");
  }
  return value;
}

async function writePatternedFile(path: string, size: number): Promise<string> {
  await mkdir(dirname(path), { recursive: true });

  return new Promise((resolve, reject) => {
    const hash = createHash("sha256");
    const stream = createWriteStream(path);
    const chunkSize = MIB;
    let written = 0;

    function writeNext() {
      while (written < size) {
        const length = Math.min(chunkSize, size - written);
        const chunk = patternedChunk(written, length);
        hash.update(chunk);
        written += length;
        if (!stream.write(chunk)) {
          stream.once("drain", writeNext);
          return;
        }
      }
      stream.end();
    }

    stream.on("error", reject);
    stream.on("finish", () => resolve(hash.digest("hex")));
    writeNext();
  });
}

function patternedChunk(startOffset: number, length: number): Buffer {
  const chunk = Buffer.allocUnsafe(length);
  for (let i = 0; i < chunk.length; i++) {
    chunk[i] = (startOffset + i) % 251;
  }
  return chunk;
}

function sha256File(path: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const hash = createHash("sha256");
    const stream = createReadStream(path);
    stream.on("data", (chunk) => hash.update(chunk));
    stream.on("error", reject);
    stream.on("end", () => resolve(hash.digest("hex")));
  });
}

function heapSampler(page: Page, samples: HeapSample[]) {
  let cdpPromise = page.context().newCDPSession(page);

  return async function sampleHeap(label: string) {
    const sample: HeapSample = { label };

    const performanceMemory = await page
      .evaluate(() => {
        const memory = (performance as Performance & { memory?: { usedJSHeapSize?: number } })
          .memory;
        return memory?.usedJSHeapSize;
      })
      .catch(() => undefined);
    if (performanceMemory !== undefined) {
      sample.performanceUsedBytes = performanceMemory;
    }

    try {
      const cdp = await cdpPromise;
      const heap = await cdp.send("Runtime.getHeapUsage");
      sample.cdpUsedBytes = heap.usedSize;
      sample.cdpTotalBytes = heap.totalSize;
    } catch {
      cdpPromise = page.context().newCDPSession(page);
    }

    samples.push(sample);
  };
}

function enforceOptionalThresholds(report: Report) {
  const maxTotalMs = optionalPositiveNumber("LARGE_E2E_MAX_TOTAL_MS");
  if (maxTotalMs !== undefined) {
    expect(report.totalMs).toBeLessThanOrEqual(maxTotalMs);
  }

  const maxHeapMiB = optionalPositiveNumber("LARGE_E2E_MAX_HEAP_MIB");
  if (maxHeapMiB !== undefined) {
    const maxHeapBytes = maxHeapMiB * MIB;
    const peak = Math.max(
      0,
      ...report.heapSamples.flatMap((sample) =>
        [sample.cdpUsedBytes, sample.performanceUsedBytes].filter(
          (value): value is number => value !== undefined,
        ),
      ),
    );
    expect(peak).toBeLessThanOrEqual(maxHeapBytes);
  }
}

function optionalPositiveNumber(name: string): number | undefined {
  const raw = process.env[name];
  if (!raw) return undefined;
  const value = Number(raw);
  if (!Number.isFinite(value) || value <= 0) {
    throw new Error(`${name} must be a positive number`);
  }
  return value;
}

function printReport(report: Report) {
  const heapLines = report.heapSamples
    .map((sample) => {
      const cdp =
        sample.cdpUsedBytes === undefined
          ? "n/a"
          : `${formatMiB(sample.cdpUsedBytes)} used / ${formatMiB(sample.cdpTotalBytes ?? 0)} total`;
      const perf =
        sample.performanceUsedBytes === undefined ? "n/a" : formatMiB(sample.performanceUsedBytes);
      return `  ${sample.label}: CDP ${cdp}, performance.memory ${perf}`;
    })
    .join("\n");

  console.log(`
Large-file E2E report
  file: ${report.fileSizeMiB} MiB (${report.fileSizeBytes} bytes)
  upload: ${formatMs(report.uploadMs)}
  reveal: ${formatMs(report.revealMs)}
  download: ${formatMs(report.downloadMs)}
  retrieve total: ${formatMs(report.retrieveTotalMs)}
  range requests: ${report.rangeRequestCount}
  total: ${formatMs(report.totalMs)}
  sha256: ${report.downloadedSha256}
  heap:
${heapLines}
`);
}

function formatMs(ms: number): string {
  return `${(ms / 1000).toFixed(2)}s`;
}

function formatMiB(bytes: number): string {
  return `${(bytes / MIB).toFixed(1)} MiB`;
}
