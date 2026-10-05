import { expect, test } from "@playwright/test";
import { expectAccessible } from "./axe";

// Screens that need a share are checked in the specs that create one, so
// this suite adds no uploads to the rate-limited API.
test.describe("Accessibility", () => {
  test("the composer, empty, with each setting open, and with files", async ({ page }) => {
    await page.goto("/share");
    await expectAccessible(page);

    await page.fill("#secret-text", "hello");
    await page.getByRole("button", { name: "Password" }).click();
    await page.getByRole("button", { name: /^Expires in/ }).click();
    await expect(page.getByRole("button", { name: "5 minutes", exact: true })).toBeVisible();
    await expectAccessible(page);

    await page.keyboard.press("Escape");
    await page.getByRole("button", { name: /^Opens/ }).click();
    await expect(page.getByRole("button", { name: /Until it expires/ })).toBeVisible();
    await expectAccessible(page);

    await page.keyboard.press("Escape");
    await page.setInputFiles('input[type="file"]', {
      name: "notes.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("notes"),
    });
    await expect(page.getByText("notes.txt")).toBeVisible();
    await expectAccessible(page);
  });

  test("open page, code entry and error pages", async ({ page }) => {
    await page.goto("/s");
    await expectAccessible(page);

    await page.getByRole("button", { name: "Enter a code" }).click();
    await expectAccessible(page);

    await page.goto("/c");
    await expect(page.getByLabel(/Type the code/)).toBeFocused();
    await expectAccessible(page);

    await page.goto("/no-such-page");
    await expect(page.locator("h1")).toHaveText("This page doesn't exist");
    await expectAccessible(page);

    await page.goto(`/s#${"A".repeat(42)}`);
    await expect(page.getByText(/This link is incomplete or damaged/)).toBeVisible();
    await expectAccessible(page);
  });
});
