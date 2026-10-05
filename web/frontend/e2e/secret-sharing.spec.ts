import { expect, type Page, test } from "@playwright/test";
import { expectAccessible } from "./axe";

/** A link the result page shows; the owner link only once its section is open. */
async function shownLink(page: Page, which: "share-link" | "owner-link"): Promise<string> {
  if (which === "owner-link") await page.getByRole("button", { name: "Owner link" }).click();
  return (await page.getByTestId(which).textContent()) ?? "";
}

test.describe("Text secret sharing", () => {
  test("create secret and retrieve via share link", async ({ page }) => {
    const secretText = `Test secret ${Date.now()}`;

    await page.goto("/share");

    await page.fill("#secret-text", secretText);
    await page.click('button[type="submit"]');

    await expect(page.getByRole("heading", { name: "Your link is ready" })).toBeVisible({
      timeout: 10000,
    });

    await expectAccessible(page);

    const shareUrl = await shownLink(page, "share-link");
    expect(shareUrl).toContain("/s#");

    // Open the link in a tab that already shows /s, as when it is pasted into
    // the address bar there: only the fragment changes.
    await page.goto("/s");
    await expect(page.locator("h1")).toHaveText("Open a secret");
    await page.goto(shareUrl);

    await expect(page.locator("h1")).toHaveText("Someone sent you a secret", { timeout: 10000 });
    await expectAccessible(page);

    await page.getByRole("button", { name: /^Reveal/ }).click();

    await expect(page.locator("h1")).toHaveText("Here's your secret", { timeout: 10000 });
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
    // Links open once unless told otherwise; the password is the one setting to turn on.
    await page.getByRole("button", { name: "Password" }).click();
    await page.fill('input[type="password"]', password);
    await expectAccessible(page);
    await page.click('button[type="submit"]');

    await expect(page.getByRole("heading", { name: "Your link is ready" })).toBeVisible({
      timeout: 10000,
    });

    const shareUrl = await shownLink(page, "share-link");
    await page.goto(shareUrl);

    await expect(page.locator("h1")).toHaveText("Someone sent you a secret", { timeout: 10000 });

    await page.getByRole("button", { name: "Unlock Share" }).click();

    await expect(page.locator("h1")).toHaveText("Enter the password", { timeout: 10000 });
    await expectAccessible(page);

    await page.fill('input[type="password"]', password);
    await page.click('button[type="submit"]');

    await expect(page.locator("h1")).toHaveText("Here's your secret", { timeout: 10000 });
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
    await expect(page.getByRole("heading", { name: "Your link is ready" })).toBeVisible({
      timeout: 10000,
    });

    const shareUrl = await shownLink(page, "share-link");
    const ownerUrl = await shownLink(page, "owner-link");
    expect(ownerUrl).toContain("!");
    await expectAccessible(page);

    // Opening a one-time secret would use it up, so the owner deletes it unopened.
    await page.goto(ownerUrl);
    await expect(page.locator("h1")).toHaveText("Your secret", { timeout: 10000 });

    await page.getByRole("button", { name: "Delete share" }).click();
    await expectAccessible(page);
    await page.getByRole("button", { name: "Delete permanently" }).click();
    await expect(page.getByRole("main").getByText("Secret deleted")).toBeVisible({
      timeout: 10000,
    });

    const recipientPage = await context.newPage();
    await recipientPage.goto(shareUrl);
    await expect(recipientPage.getByText(/It was opened already, or it expired\./)).toBeVisible({
      timeout: 10000,
    });
  });
});
