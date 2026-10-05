import AxeBuilder from "@axe-core/playwright";
import { expect, type Page } from "@playwright/test";

/** WCAG 2.2 at levels A and AA: the bar every screen has to meet. */
const WCAG_AA = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"];

async function expectNoViolations(page: Page, scheme: "light" | "dark") {
  await page.emulateMedia({ colorScheme: scheme });
  const { violations } = await new AxeBuilder({ page }).withTags(WCAG_AA).analyze();
  const found = violations.flatMap((violation) =>
    violation.nodes.map((node) => `${scheme}: ${violation.id} at ${node.target.join(" ")}`),
  );
  expect(found, "accessibility violations").toEqual([]);
}

/**
 * Runs axe on the page as it is, in the light and the dark theme, and ends in
 * the light one. Transitions are switched off first: axe would otherwise
 * measure colours halfway through a fade.
 */
export async function expectAccessible(page: Page) {
  await page.addStyleTag({
    content: "*, *::before, *::after { transition: none !important; animation: none !important; }",
  });
  await expectNoViolations(page, "light");
  await expectNoViolations(page, "dark");
  await page.emulateMedia({ colorScheme: "light" });
}
