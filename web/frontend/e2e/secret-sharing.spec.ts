import { expect, test } from "@playwright/test";

test.describe("Text secret sharing", () => {
  test("create secret and retrieve via share link", async ({ page }) => {
    const secretText = `Test secret ${Date.now()}`;

    await page.goto("/share");

    await page.fill("#secret-text", secretText);
    await page.click('button[type="submit"]');

    await expect(page.getByRole("main").getByText("Secret created")).toBeVisible({
      timeout: 10000,
    });

    const shareInput = page.locator("input[readonly]").first();
    const shareUrl = await shareInput.inputValue();
    expect(shareUrl).toContain("/s#");

    await page.goto(shareUrl);

    await expect(page.locator("h1")).toHaveText("Secret Ready", { timeout: 10000 });

    await page.getByRole("button", { name: "Reveal Secret" }).click();

    await expect(page.locator("h1")).toHaveText("Secret", { timeout: 10000 });

    const decryptedText = await page.locator("pre").textContent();
    expect(decryptedText).toBe(secretText);
  });

  test("create password-protected secret and retrieve", async ({ page }) => {
    const secretText = `Password secret ${Date.now()}`;
    const password = "testpassword123";

    await page.goto("/share");

    await page.fill("#secret-text", secretText);
    await page.getByRole("switch", { name: /Password protection/ }).click();
    await page.fill('input[type="password"]', password);
    await page.click('button[type="submit"]');

    await expect(page.getByRole("main").getByText("Secret created")).toBeVisible({
      timeout: 10000,
    });

    const shareUrl = await page.locator("input[readonly]").first().inputValue();
    await page.goto(shareUrl);

    await expect(page.locator("h1")).toHaveText("Secret Ready", { timeout: 10000 });

    await page.getByRole("button", { name: "Enter Password" }).click();

    await expect(page.locator("h1")).toHaveText("Password Required", { timeout: 10000 });

    await page.fill('input[type="password"]', password);
    await page.click('button[type="submit"]');

    await expect(page.locator("h1")).toHaveText("Secret", { timeout: 10000 });
    const decryptedText = await page.locator("pre").textContent();
    expect(decryptedText).toBe(secretText);
  });

  test("owner link can delete a text secret before recipients retrieve it", async ({
    page,
    context,
  }) => {
    const secretText = `Delete secret ${Date.now()}`;

    await page.goto("/share");
    await page.fill("#secret-text", secretText);
    await page.click('button[type="submit"]');
    await expect(page.getByRole("main").getByText("Secret created")).toBeVisible({
      timeout: 10000,
    });

    const shareUrl = await page.locator("input[readonly]").first().inputValue();
    const ownerUrl = await page.locator("input[readonly]").nth(1).inputValue();
    expect(ownerUrl).toContain("!");

    await page.goto(ownerUrl);
    await expect(page.locator("h1")).toHaveText("Secret Ready", { timeout: 10000 });
    await page.getByRole("button", { name: "Reveal Secret" }).click();
    await expect(page.locator("h1")).toHaveText("Secret", { timeout: 10000 });

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
