import { expect, test } from "@playwright/test";

test.describe("Text secret sharing", () => {
  test("create secret and retrieve via share link", async ({ page }) => {
    const secretText = `Test secret ${Date.now()}`;

    // Navigate to share page
    await page.goto("/");
    await expect(page.locator("h1")).toHaveText("Share a Secret");

    // Fill in the secret form
    await page.fill("#secret-text", secretText);
    await page.click('button[type="submit"]');

    // Wait for result
    await expect(page.locator("text=Secret created!")).toBeVisible({ timeout: 10000 });

    // Get the share link
    const shareInput = page.locator("input[readonly]");
    const shareUrl = await shareInput.inputValue();
    expect(shareUrl).toContain("/s#");

    // Navigate to the share link
    await page.goto(shareUrl);

    // Should show confirm screen first
    await expect(page.locator("h1")).toHaveText("Secret Ready", { timeout: 10000 });

    // Click reveal
    await page.click("text=Reveal Secret");

    // Wait for decryption
    await expect(page.locator("h1")).toHaveText("Secret", { timeout: 10000 });

    // Verify decrypted text matches
    const decryptedText = await page.locator("pre").textContent();
    expect(decryptedText).toBe(secretText);
  });

  test("create password-protected secret and retrieve", async ({ page }) => {
    const secretText = `Password secret ${Date.now()}`;
    const password = "testpassword123";

    await page.goto("/");

    await page.fill("#secret-text", secretText);
    await page.click("text=Add password protection");
    await page.fill('input[type="password"]', password);
    await page.click('button[type="submit"]');

    await expect(page.locator("text=Secret created!")).toBeVisible({ timeout: 10000 });

    const shareUrl = await page.locator("input[readonly]").inputValue();
    await page.goto(shareUrl);

    // Should show confirm screen with password required
    await expect(page.locator("h1")).toHaveText("Secret Ready", { timeout: 10000 });

    // Click reveal - should go to password prompt
    await page.click("text=Reveal Secret");

    // Should prompt for password
    await expect(page.locator("h1")).toHaveText("Password Required", { timeout: 10000 });

    await page.fill('input[type="password"]', password);
    await page.click('button[type="submit"]');

    // Wait for decryption
    await expect(page.locator("h1")).toHaveText("Secret", { timeout: 10000 });
    const decryptedText = await page.locator("pre").textContent();
    expect(decryptedText).toBe(secretText);
  });
});
