import { expect, test } from "@playwright/test";
import { expectAccessible } from "./axe";

test.describe("Text secret sharing", () => {
  test("create secret and retrieve via share link", async ({ page }) => {
    const secretText = `Test secret ${Date.now()}`;

    await page.goto("/share");

    await page.fill("#secret-text", secretText);
    await page.click('button[type="submit"]');

    await expect(page.getByRole("main").getByText("Secure link created")).toBeVisible({
      timeout: 10000,
    });

    await expectAccessible(page);

    const shareInput = page.locator("input[readonly]").first();
    const shareUrl = await shareInput.inputValue();
    expect(shareUrl).toContain("/s#");

    // Open the link in a tab that already shows /s, as when it is pasted into
    // the address bar there: only the fragment changes.
    await page.goto("/s");
    await expect(page.locator("h1")).toHaveText("Open a Share");
    await page.goto(shareUrl);

    await expect(page.locator("h1")).toHaveText("Text Share", { timeout: 10000 });
    await expectAccessible(page);

    await page.getByRole("button", { name: "Reveal Text" }).click();

    await expect(page.locator("h1")).toHaveText("Decrypted Text", { timeout: 10000 });
    await expectAccessible(page);

    const decryptedText = await page.locator("pre").textContent();
    expect(decryptedText).toBe(secretText);
  });

  test("password-protected one-time secret: retrieve it, then get asked before leaving", async ({
    page,
  }) => {
    const secretText = `Password secret ${Date.now()}`;
    const password = "testpassword123";

    await page.goto("/share");

    await page.fill("#secret-text", secretText);
    await page.getByRole("switch", { name: /Burn after reading/ }).click();
    await page.getByRole("switch", { name: /Password protection/ }).click();
    await page.fill('input[type="password"]', password);
    await page.click('button[type="submit"]');

    await expect(page.getByRole("main").getByText("Secure link created")).toBeVisible({
      timeout: 10000,
    });

    const shareUrl = await page.locator("input[readonly]").first().inputValue();
    await page.goto(shareUrl);

    await expect(page.locator("h1")).toHaveText("Text Share", { timeout: 10000 });

    await page.getByRole("button", { name: "Unlock Share" }).click();

    await expect(page.locator("h1")).toHaveText("Unlock Share", { timeout: 10000 });
    await expectAccessible(page);

    await page.fill('input[type="password"]', password);
    await page.click('button[type="submit"]');

    await expect(page.locator("h1")).toHaveText("Decrypted Text", { timeout: 10000 });
    const decryptedText = await page.locator("pre").textContent();
    expect(decryptedText).toBe(secretText);

    // The share is gone from the server now: leaving asks first, also
    // through the app's own links.
    await expect(page.getByText(/this page has the only copy/)).toBeVisible();
    // The dialog blocks the click until it is answered: stay on the page.
    const [leaving] = await Promise.all([
      page.waitForEvent("dialog").then(async (dialog) => {
        await dialog.dismiss();
        return dialog;
      }),
      page.getByRole("link", { name: "Share" }).click(),
    ]);
    expect(leaving.type()).toBe("beforeunload");
    await expect(page.locator("pre")).toHaveText(secretText);
  });

  test("owner link can delete a text secret before recipients retrieve it", async ({
    page,
    context,
  }) => {
    const secretText = `Delete secret ${Date.now()}`;

    await page.goto("/share");
    await page.fill("#secret-text", secretText);
    await page.click('button[type="submit"]');
    await expect(page.getByRole("main").getByText("Secure link created")).toBeVisible({
      timeout: 10000,
    });

    const shareUrl = await page.locator("input[readonly]").first().inputValue();
    const ownerUrl = await page.locator("input[readonly]").nth(1).inputValue();
    expect(ownerUrl).toContain("!");

    await page.goto(ownerUrl);
    await expect(page.locator("h1")).toHaveText("Text Share", { timeout: 10000 });
    await page.getByRole("button", { name: "Reveal Text" }).click();
    await expect(page.locator("h1")).toHaveText("Decrypted Text", { timeout: 10000 });

    await page.getByRole("button", { name: "Delete share" }).click();
    await expectAccessible(page);
    await page.getByRole("button", { name: "Delete permanently" }).click();
    await expect(page.getByRole("main").getByText("Share deleted")).toBeVisible({
      timeout: 10000,
    });

    const recipientPage = await context.newPage();
    await recipientPage.goto(shareUrl);
    await expect(recipientPage.getByText(/This share has expired or was deleted\./)).toBeVisible({
      timeout: 10000,
    });
  });
});
