## 1. Toast Notifications

- [x] 1.1 Install sonner: `npm install sonner` in `web/frontend/`
- [x] 1.2 Add `<Toaster />` to `App.tsx` with theme-aware config (respects dark mode)
- [x] 1.3 Add success toast to SharePage on secret creation
- [x] 1.4 Add success toast to FilePage on file upload
- [x] 1.5 Replace inline error displays with error toasts across SharePage, FilePage, LoginPage, RegisterPage
- [x] 1.6 Add toast to SecretResult on copy-to-clipboard (replace the "Copied!" button state)
- [x] 1.7 Add success/info toasts to auth actions (login success, logout)

## 2. Spinner Component

- [x] 2.1 Create `components/Spinner.tsx` using Tailwind `animate-spin` on an SVG circle, with size prop (sm/md/lg)
- [x] 2.2 Add spinner to SecretForm submit button during encryption
- [x] 2.3 Add spinner to RetrievePage during decryption (replace text-only loading)
- [x] 2.4 Add spinner to FileUpload button during upload
- [x] 2.5 Add spinner to LoginPage and RegisterPage submit buttons during submission

## 3. Form Validation

- [x] 3.1 Create `lib/validation.ts` with validators: `validateEmail`, `validatePassword` (min 8 chars), `validateRequired`
- [x] 3.2 Add field-level validation to LoginPage (email format, password required) with error messages below inputs, validate on blur
- [x] 3.3 Add field-level validation to RegisterPage (email format, password min 8, display name required) with error messages below inputs
- [x] 3.4 Add validation to SecretForm: require non-empty text, require password when password-protected is enabled

## 4. Copy-to-Clipboard Enhancement

- [x] 4.1 Add copy button to RetrievePage for decrypted secret text (next to the text display area)

## 5. Dark Mode

- [x] 5.1 Add dark mode CSS custom variant to `index.css`: `@custom-variant dark (&:where(.dark, .dark *))`
- [x] 5.2 Create `hooks/useTheme.ts` with theme state (light/dark/system), localStorage persistence, and `prefers-color-scheme` media query listener; applies/removes `.dark` class on `<html>`
- [x] 5.3 Add theme toggle button to Layout header (sun/moon icon, cycles light → dark → system)
- [x] 5.4 Add `dark:` variants to Layout.tsx (header, footer, nav links)
- [x] 5.5 Add `dark:` variants to all page components (SharePage, RetrievePage, FilePage, LoginPage, RegisterPage, HistoryPage, NotFoundPage)
- [x] 5.6 Add `dark:` variants to all shared components (SecretForm, SecretResult, FileUpload, ExpirationPicker, Spinner)
- [x] 5.7 Add `dark:` variants to form inputs across all components (backgrounds, borders, text, focus rings)

## 6. Responsive Design

- [x] 6.1 Add hamburger menu to Layout.tsx: hide nav links behind a menu button on `md:` breakpoint, show dropdown panel on tap
- [x] 6.2 Convert HistoryPage table to card layout on mobile: show table on `md:` and above, stacked cards below

## 7. Playwright E2E Tests

- [x] 7.1 Install Playwright: `npm install -D @playwright/test` in `web/frontend/`, create `playwright.config.ts`
- [x] 7.2 Create e2e test: text secret create → copy share link → open → decrypt → verify plaintext matches
- [x] 7.3 Create e2e test: register → login → verify username in header

## 8. Verification

- [x] 8.1 Verify frontend builds without errors: `npm run build`
- [x] 8.2 Verify all lint checks pass (pre-existing AuthContext lint warning, not caused by Phase 8)
