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
  await expect(page.getByRole("main").getByText("Secret created")).toBeVisible({
    timeout: 10000,
  });

  return {
    shareUrl: await page.locator("input[readonly]").first().inputValue(),
    ownerUrl: await page.locator("input[readonly]").nth(1).inputValue(),
  };
}

async function revealBundle(page: Page, shareUrl: string, password?: string) {
  await page.goto(shareUrl);
  await expect(page.locator("h1")).toHaveText("Secret Ready", { timeout: 10000 });

  if (password) {
    await page.getByRole("button", { name: "Enter Password" }).click();
    await expect(page.locator("h1")).toHaveText("Password Required", { timeout: 10000 });
    await page.fill('input[type="password"]', password);
    await page.click('button[type="submit"]');
  } else {
    await page.getByRole("button", { name: /Download/ }).click();
  }
}

async function downloadTextFile(
  page: Page,
  rowIndex: number,
  filename: string,
  outputPath: string,
): Promise<string> {
  const downloadPromise = page.waitForEvent("download");
  await page
    .getByTestId(`bundle-file-${rowIndex}`)
    .getByRole("button", { name: "Download" })
    .click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe(filename);
  await download.saveAs(outputPath);
  return readFile(outputPath, "utf8");
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

    await expect(page.locator("h1")).toHaveText("File Ready", { timeout: 10000 });
    await expect(page.getByTestId("bundle-file-0").getByText(file.name)).toBeVisible();

    const downloaded = await downloadTextFile(page, 0, file.name, testInfo.outputPath(file.name));
    expect(downloaded).toBe(file.contents);
  });

  test("creates and retrieves a multi-file bundle", async ({ page }, testInfo) => {
    const files = [
      { name: "alpha.txt", mimeType: "text/plain", contents: "alpha contents" },
      { name: "bravo.txt", mimeType: "text/plain", contents: "bravo contents" },
    ];

    const shareUrl = await createFileSecret(page, files);
    await revealBundle(page, shareUrl);

    await expect(page.locator("h1")).toHaveText("Files Ready", { timeout: 10000 });
    await expect(page.getByText("alpha.txt")).toBeVisible();
    await expect(page.getByText("bravo.txt")).toBeVisible();

    for (const [index, file] of files.entries()) {
      const downloaded = await downloadTextFile(
        page,
        index,
        file.name,
        testInfo.outputPath(file.name),
      );
      expect(downloaded).toBe(file.contents);
    }
  });

  test("downloads all files from a bundle", async ({ page }, testInfo) => {
    const files = [
      { name: "all-alpha.txt", mimeType: "text/plain", contents: "download all alpha" },
      { name: "all-bravo.txt", mimeType: "text/plain", contents: "download all bravo" },
    ];

    const shareUrl = await createFileSecret(page, files);
    await revealBundle(page, shareUrl);
    await expect(page.locator("h1")).toHaveText("Files Ready", { timeout: 10000 });

    const downloads: Download[] = [];
    page.on("download", (download) => downloads.push(download));
    await page.getByRole("button", { name: "Download All" }).click();
    await expect.poll(() => downloads.length, { timeout: 10000 }).toBe(files.length);

    for (const [index, file] of files.entries()) {
      const download = downloads[index];
      expect(download.suggestedFilename()).toBe(file.name);
      const outputPath = testInfo.outputPath(file.name);
      await download.saveAs(outputPath);
      await expect(readFile(outputPath, "utf8")).resolves.toBe(file.contents);
    }
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

    await expect(page.locator("h1")).toHaveText("File Ready", { timeout: 10000 });
    const downloaded = await downloadTextFile(page, 0, file.name, testInfo.outputPath(file.name));
    expect(downloaded).toBe(file.contents);
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
    await expect(page.locator("h1")).toHaveText("File Ready", { timeout: 10000 });

    const secondPage = await context.newPage();
    await secondPage.goto(shareUrl);
    await expect(secondPage.getByText("This secret has expired or does not exist.")).toBeVisible({
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
    await expect(page.locator("h1")).toHaveText("File Ready", { timeout: 10000 });

    await page.getByRole("button", { name: "Delete this secret" }).click();
    await expect(page.getByRole("main").getByText("Secret deleted")).toBeVisible({
      timeout: 10000,
    });

    const recipientPage = await context.newPage();
    await recipientPage.goto(shareUrl);
    await expect(recipientPage.getByText("This secret has expired or does not exist.")).toBeVisible(
      { timeout: 10000 },
    );
  });
});
