import { expect, type Page, test } from "@playwright/test";
import { expectAccessible } from "./axe";

const CODE = /^\d{1,3}-[a-z]+-[a-z]+$/;

async function shownCode(sender: Page): Promise<string> {
  const code = sender.getByText(CODE);
  await expect(code).toBeVisible({ timeout: 10000 });
  return (await code.textContent()) ?? "";
}

/** Same number, other words: a code that reaches the transfer but cannot match. */
function wrongCode(code: string): string {
  const [nameplate, first] = code.split("-");
  return first === "acid" ? `${nameplate}-apple-atlas` : `${nameplate}-acid-robe`;
}

test.describe("Short-code transfer", () => {
  test("a wrong code fails on both sides, the right one opens the share", async ({
    page: sender,
    browser,
  }) => {
    const secretText = `Code transfer ${Date.now()}`;

    await sender.goto("/share");
    await sender.fill("#secret-text", secretText);
    await sender.click('button[type="submit"]');
    await expect(sender.getByRole("main").getByText("Secure link created")).toBeVisible({
      timeout: 10000,
    });
    await sender.getByRole("button", { name: "Send with a code" }).click();
    const firstCode = await shownCode(sender);
    await expect(sender.getByText(/go to .+\/c and type/)).toBeVisible();
    await expectAccessible(sender);

    const receiverContext = await browser.newContext();
    try {
      const receiver = await receiverContext.newPage();
      // The address the sender's panel names: code entry, ready to type.
      await receiver.goto("/c");
      const codeField = receiver.getByLabel(/Type the code/);
      await expect(codeField).toBeFocused();

      await test.step("a wrong code", async () => {
        await codeField.fill(wrongCode(firstCode));
        await receiver.getByRole("button", { name: "Receive share" }).click();
        await expect(receiver.getByText("The code didn't match. Ask for a new code.")).toBeVisible({
          timeout: 15000,
        });
        await expect(
          sender.getByText("The code didn't match. Start again for a new code."),
        ).toBeVisible({ timeout: 15000 });
      });

      await test.step("the right code", async () => {
        await sender.getByRole("button", { name: "New code" }).click();
        const code = await shownCode(sender);
        expect(code).not.toBe(firstCode);

        await codeField.fill(code);
        await receiver.getByRole("button", { name: "Receive share" }).click();

        await expect(receiver.locator("h1")).toHaveText("Text Share", { timeout: 15000 });
        await expect(
          sender.getByText("Sent. The other device is opening the share."),
        ).toBeVisible();
        await receiver.getByRole("button", { name: "Reveal Text" }).click();
        await expect(receiver.locator("pre")).toHaveText(secretText, { timeout: 10000 });
      });
    } finally {
      await receiverContext.close();
    }
  });
});
