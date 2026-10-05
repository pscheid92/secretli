import { mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { chromium, expect, test } from "@playwright/test";
import QRCode from "qrcode";

const WIDTH = 640;
const HEIGHT = 480;
const QUIET_ZONE = 4;

/**
 * Writes `text` as a QR code into a Y4M video, the format Chromium's fake
 * camera plays. The code lives in the luma plane; chroma stays neutral grey.
 */
async function writeQRVideo(text: string): Promise<string> {
  const { modules } = QRCode.create(text, { errorCorrectionLevel: "M" });
  const scale = Math.floor((HEIGHT - 40) / (modules.size + QUIET_ZONE * 2));
  const left = Math.floor((WIDTH - modules.size * scale) / 2);
  const upper = Math.floor((HEIGHT - modules.size * scale) / 2);

  const luma = Buffer.alloc(WIDTH * HEIGHT, 255);
  for (let row = 0; row < modules.size; row++) {
    for (let col = 0; col < modules.size; col++) {
      if (!modules.get(row, col)) continue;
      for (let y = 0; y < scale; y++) {
        const start = (upper + row * scale + y) * WIDTH + left + col * scale;
        luma.fill(0, start, start + scale);
      }
    }
  }
  const chroma = Buffer.alloc((WIDTH / 2) * (HEIGHT / 2) * 2, 128);
  const frame = Buffer.concat([Buffer.from("FRAME\n"), luma, chroma]);
  const header = Buffer.from(`YUV4MPEG2 W${WIDTH} H${HEIGHT} F10:1 Ip A1:1 C420jpeg\n`);

  const file = join(await mkdtemp(join(tmpdir(), "secretli-qr-")), "qr.y4m");
  await writeFile(file, Buffer.concat([header, frame, frame, frame]));
  return file;
}

test.describe("QR code scanning", () => {
  test("opens a share by scanning its QR code with the camera", async ({ page, baseURL }) => {
    await page.goto("/share");
    await page.fill("#secret-text", `QR secret ${Date.now()}`);
    await page.click('button[type="submit"]');
    await expect(page.getByRole("heading", { name: "Share is ready" })).toBeVisible({
      timeout: 10000,
    });
    const shareUrl = await page.locator("input[readonly]").first().inputValue();

    // The receiver is a second browser whose only camera shows the QR code.
    // Headless Chromium also needs the fake permission UI, or getUserMedia
    // rejects with NotSupportedError.
    const video = await writeQRVideo(shareUrl);
    const receiverBrowser = await chromium.launch({
      args: [
        "--use-fake-ui-for-media-stream",
        "--use-fake-device-for-media-stream",
        `--use-file-for-fake-video-capture=${video}`,
      ],
    });
    try {
      for (const decoder of ["jsQR", "built-in"] as const) {
        await test.step(`scan with the ${decoder} decoder`, async () => {
          const context = await receiverBrowser.newContext({ baseURL });
          if (decoder === "jsQR") {
            await context.addInitScript(() => {
              delete (window as { BarcodeDetector?: unknown }).BarcodeDetector;
            });
          }
          const receiver = await context.newPage();
          await receiver.goto("/s");
          // Chromium has the built-in detector only on macOS, ChromeOS and Android.
          if (decoder === "jsQR" || (await receiver.evaluate(() => "BarcodeDetector" in window))) {
            await receiver.getByRole("button", { name: "Scan QR code" }).click();
            // The share details decrypt only with the scanned key.
            await expect(receiver.locator("h1")).toHaveText("Text Share", { timeout: 15000 });
          }
          await context.close();
        });
      }
    } finally {
      await receiverBrowser.close();
    }
  });
});
