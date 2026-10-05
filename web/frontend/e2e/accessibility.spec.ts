import { expect, test } from "@playwright/test";
import { expectAccessible } from "./axe";

// Screens that need a share are checked in the specs that create one, so
// this suite adds no uploads to the rate-limited API.
test.describe("Accessibility", () => {
  test("create pages, empty and filled in", async ({ page }) => {
    await page.goto("/share");
    await expectAccessible(page);

    await page.fill("#secret-text", "hello");
    await page.getByRole("switch", { name: /Burn after reading/ }).click();
    await page.getByRole("switch", { name: /Password protection/ }).click();
    await expectAccessible(page);

    await page.goto("/file");
    await page.setInputFiles('input[type="file"]', {
      name: "notes.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("notes"),
    });
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
    await expect(page.locator("h1")).toHaveText("This page doesn't exist.");
    await expectAccessible(page);

    await page.goto(`/s#${"A".repeat(42)}`);
    await expect(page.getByText(/This link is incomplete or damaged/)).toBeVisible();
    await expectAccessible(page);
  });
});
