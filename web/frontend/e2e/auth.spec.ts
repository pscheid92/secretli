import { test, expect } from '@playwright/test'

test.describe('Authentication', () => {
  test('register and login', async ({ page }) => {
    const email = `test-${Date.now()}@example.com`
    const password = 'testpassword123'
    const displayName = 'Test User'

    // Register
    await page.goto('/register')
    await expect(page.locator('h1')).toHaveText('Create Account')

    await page.fill('#display-name', displayName)
    await page.fill('#email', email)
    await page.fill('#password', password)
    await page.click('button[type="submit"]')

    // Should redirect to home and show user name in header
    await expect(page).toHaveURL('/', { timeout: 10000 })
    await expect(page.locator('header')).toContainText(displayName)

    // Logout
    await page.click('text=Logout')

    // Login
    await page.goto('/login')
    await expect(page.locator('h1')).toHaveText('Sign In')

    await page.fill('#email', email)
    await page.fill('#password', password)
    await page.click('button[type="submit"]')

    // Should redirect and show user name
    await expect(page).toHaveURL('/', { timeout: 10000 })
    await expect(page.locator('header')).toContainText(displayName)

    // Can access history
    await page.click('text=History')
    await expect(page.locator('h1')).toHaveText('Secret History')
  })
})
