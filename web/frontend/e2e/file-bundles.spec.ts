import { readFile } from "node:fs/promises";
import { type Download, expect, type Page, test } from "@playwright/test";

interface TestFile {
  name: string;
  mimeType: string;
  contents: string;
}

async function createFileSecret(
  page: Page,
  files: TestFile[],
  options: { password?: string; burnAfterRead?: boolean } = {},
): Promise<string> {
  const { shareUrl } = await createFileSecretLinks(page, files, options);
  return shareUrl;
}

async function createFileSecretLinks(
  page: Page,
  files: TestFile[],
  options: { password?: string; burnAfterRead?: boolean } = {},
): Promise<{ shareUrl: string; ownerUrl: string }> {
  await page.goto("/file");
  await page.setInputFiles(
    'input[type="file"]',
    files.map((file) => ({
      name: file.name,
      mimeType: file.mimeType,
      buffer: Buffer.from(file.contents),
    })),
  );

  if (options.burnAfterRead) {
    await page.getByRole("switch", { name: /Burn after reading/ }).click();
  }
  if (options.password) {
    await page.getByRole("switch", { name: /Password protection/ }).click();
    await page.fill('input[type="password"]', options.password);
  }

  await page.click('button[type="submit"]');
  await expect(page.getByRole("main").getByText("Secure link created")).toBeVisible({
    timeout: 10000,
  });

  return {
    shareUrl: await page.locator("input[readonly]").first().inputValue(),
    ownerUrl: await page.locator("input[readonly]").nth(1).inputValue(),
  };
}

async function revealBundle(page: Page, shareUrl: string, password?: string) {
  await page.goto(shareUrl);
  await expect(page.locator("h1")).toHaveText("File Share", { timeout: 10000 });

  if (password) {
    await page.getByRole("button", { name: "Unlock Share" }).click();
    await expect(page.locator("h1")).toHaveText("Unlock Share", { timeout: 10000 });
    await page.fill('input[type="password"]', password);
    await page.click('button[type="submit"]');
  } else {
    await page.getByRole("button", { name: /Prepare Download/ }).click();
  }
}

async function downloadBundleFiles(
  page: Page,
  files: TestFile[],
  outputPath: (filename: string) => string,
) {
  const downloads: Download[] = [];
  page.on("download", (download) => downloads.push(download));

  await page
    .getByRole("button", { name: files.length > 1 ? "Download Files" : "Download File" })
    .click();
  await expect.poll(() => downloads.length, { timeout: 10000 }).toBe(files.length);

  for (const [index, file] of files.entries()) {
    const download = downloads[index];
    expect(download.suggestedFilename()).toBe(file.name);
    const path = outputPath(file.name);
    await download.saveAs(path);
    await expect(readFile(path, "utf8")).resolves.toBe(file.contents);
  }
}

test.describe("File bundle sharing", () => {
  test("creates and retrieves a single-file bundle", async ({ page }, testInfo) => {
    const file = {
      name: "single-note.txt",
      mimeType: "text/plain",
      contents: `single file ${Date.now()}`,
    };

    const shareUrl = await createFileSecret(page, [file]);
    await revealBundle(page, shareUrl);

    await expect(page.locator("h1")).toHaveText("Download File", { timeout: 10000 });
    await expect(page.getByTestId("bundle-file-0").getByText(file.name)).toBeVisible();

    await downloadBundleFiles(page, [file], (filename) => testInfo.outputPath(filename));
  });

  test("creates and downloads a multi-file bundle", async ({ page }, testInfo) => {
    const files = [
      { name: "all-alpha.txt", mimeType: "text/plain", contents: "download all alpha" },
      { name: "all-bravo.txt", mimeType: "text/plain", contents: "download all bravo" },
    ];

    const shareUrl = await createFileSecret(page, files);
    await revealBundle(page, shareUrl);
    await expect(page.locator("h1")).toHaveText("Download Files", { timeout: 10000 });
    await expect(page.getByText("all-alpha.txt")).toBeVisible();
    await expect(page.getByText("all-bravo.txt")).toBeVisible();

    await downloadBundleFiles(page, files, (filename) => testInfo.outputPath(filename));
  });

  test("requires password before listing a protected bundle", async ({ page }, testInfo) => {
    const password = "bundle-password";
    const file = {
      name: "protected.txt",
      mimeType: "text/plain",
      contents: "protected bundle contents",
    };

    const shareUrl = await createFileSecret(page, [file], { password });
    await revealBundle(page, shareUrl, password);

    await expect(page.locator("h1")).toHaveText("Download File", { timeout: 10000 });
    await downloadBundleFiles(page, [file], (filename) => testInfo.outputPath(filename));
  });

  test("burn-after-read bundle cannot start a second retrieval session", async ({
    page,
    context,
  }) => {
    const file = {
      name: "burn.txt",
      mimeType: "text/plain",
      contents: "burn once",
    };

    const shareUrl = await createFileSecret(page, [file], { burnAfterRead: true });
    await revealBundle(page, shareUrl);
    await expect(page.locator("h1")).toHaveText("Download File", { timeout: 10000 });

    const secondPage = await context.newPage();
    await secondPage.goto(shareUrl);
    await expect(secondPage.getByText("This share has expired or does not exist.")).toBeVisible({
      timeout: 10000,
    });
  });

  test("owner link can delete a bundle before recipients retrieve it", async ({
    page,
    context,
  }) => {
    const file = {
      name: "delete-me.txt",
      mimeType: "text/plain",
      contents: "bundle to delete",
    };

    const { shareUrl, ownerUrl } = await createFileSecretLinks(page, [file]);
    await revealBundle(page, ownerUrl);
    await expect(page.locator("h1")).toHaveText("Download File", { timeout: 10000 });

    await page.getByRole("button", { name: "Delete share" }).click();
    await expect(page.getByRole("main").getByText("Share deleted")).toBeVisible({
      timeout: 10000,
    });

    const recipientPage = await context.newPage();
    await recipientPage.goto(shareUrl);
    await expect(recipientPage.getByText("This share has expired or does not exist.")).toBeVisible({
      timeout: 10000,
    });
  });
});
