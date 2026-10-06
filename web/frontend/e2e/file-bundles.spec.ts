import { readFile } from "node:fs/promises";
import { type Download, expect, type Page, test } from "@playwright/test";
import { expectAccessible } from "./axe";

interface TestFile {
  name: string;
  mimeType: string;
  contents: string;
}

async function createFileSecret(
  page: Page,
  files: TestFile[],
  options: { password?: string } = {},
): Promise<string> {
  const { shareUrl } = await createFileSecretLinks(page, files, options);
  return shareUrl;
}

/** Links open once by default, so every bundle made here is one-time. */
async function createFileSecretLinks(
  page: Page,
  files: TestFile[],
  options: { password?: string } = {},
): Promise<{ shareUrl: string; ownerUrl: string }> {
  await page.goto("/share");
  await page.setInputFiles(
    'input[type="file"]',
    files.map((file) => ({
      name: file.name,
      mimeType: file.mimeType,
      buffer: Buffer.from(file.contents),
    })),
  );

  if (options.password) {
    await page.getByRole("button", { name: "Password" }).click();
    await page.fill('input[type="password"]', options.password);
  }

  await page.click('button[type="submit"]');
  await expect(page.getByRole("heading", { name: "Your link is ready" })).toBeVisible({
    timeout: 10000,
  });

  const shareUrl = (await page.getByTestId("share-link").textContent()) ?? "";
  await page.getByRole("button", { name: "Owner link" }).click();
  const ownerUrl = (await page.getByTestId("owner-link").textContent()) ?? "";
  return { shareUrl, ownerUrl };
}

async function revealBundle(page: Page, shareUrl: string, password?: string) {
  await page.goto(shareUrl);
  await expect(page.locator("h1")).toHaveText(/Someone sent you a secret|Your secret/, {
    timeout: 10000,
  });

  if (password) await page.fill('input[type="password"]', password);
  await page.getByRole("button", { name: "Show the files" }).click();
}

async function downloadBundleFiles(
  page: Page,
  files: TestFile[],
  outputPath: (filename: string) => string,
) {
  const downloads: Download[] = [];
  page.on("download", (download) => downloads.push(download));

  await page
    .getByRole("button", { name: files.length > 1 ? "Download files" : "Download file" })
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

    await expect(page.locator("h1")).toHaveText("Here's your file", { timeout: 10000 });
    await expect(page.getByTestId("bundle-file-0").getByText(file.name)).toBeVisible();
    await expectAccessible(page);

    await downloadBundleFiles(page, [file], (filename) => testInfo.outputPath(filename));
  });

  test("creates and downloads a multi-file bundle", async ({ page }, testInfo) => {
    const files = [
      { name: "all-alpha.txt", mimeType: "text/plain", contents: "download all alpha" },
      { name: "all-bravo.txt", mimeType: "text/plain", contents: "download all bravo" },
    ];

    const shareUrl = await createFileSecret(page, files);
    await revealBundle(page, shareUrl);
    await expect(page.locator("h1")).toHaveText("Here are your files", { timeout: 10000 });
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

    await expect(page.locator("h1")).toHaveText("Here's your file", { timeout: 10000 });
    await downloadBundleFiles(page, [file], (filename) => testInfo.outputPath(filename));
  });

  test("a one-time bundle cannot start a second retrieval session", async ({ page, context }) => {
    const file = {
      name: "burn.txt",
      mimeType: "text/plain",
      contents: "burn once",
    };

    const shareUrl = await createFileSecret(page, [file]);
    await revealBundle(page, shareUrl);
    await expect(page.locator("h1")).toHaveText("Here's your file", { timeout: 10000 });

    const secondPage = await context.newPage();
    await secondPage.goto(shareUrl);
    await expect(secondPage.getByText(/^Someone opened it today at /)).toBeVisible({
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
    // Opening a one-time bundle would use it up, so the owner deletes it unopened.
    await page.goto(ownerUrl);
    await expect(page.locator("h1")).toHaveText("Your secret", { timeout: 10000 });

    await page.getByRole("button", { name: "Delete it now" }).click();
    await page.getByRole("button", { name: "Delete permanently" }).click();
    await expect(page.getByRole("main").getByText("Secret deleted")).toBeVisible({
      timeout: 10000,
    });

    const recipientPage = await context.newPage();
    await recipientPage.goto(shareUrl);
    await expect(recipientPage.getByText(/^The sender deleted it today at /)).toBeVisible({
      timeout: 10000,
    });
  });
});
