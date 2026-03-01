## Context

The React frontend (React 19, TypeScript, Vite 7, Tailwind CSS 4) is functionally complete across 7 pages with TanStack Query for server state and AuthContext for auth. Current UX gaps: inline-only error feedback, text-only loading states, minimal HTML5 form validation, no dark mode, and no tests.

Key current state:
- Copy-to-clipboard exists on SecretResult but not on retrieved text
- Loading uses text like "Encrypting..." / "Decrypting..." — no spinners
- Forms rely on HTML5 `required`/`minLength` — no real-time feedback
- Light-only theme with hardcoded `bg-gray-50`, `text-gray-900`, etc.
- Mobile layout is decent (flexbox, max-width constraints) but nav needs hamburger menu and history table needs card view
- No test framework installed

## Goals / Non-Goals

**Goals:**
- Add toast notifications for all user-visible actions (create, copy, delete, auth, errors)
- Replace text loading indicators with spinner components
- Add client-side form validation with field-level error messages
- Support dark mode with system preference detection and manual toggle
- Improve responsive layout for mobile (nav menu, history cards)
- Add Playwright e2e tests for the core share-and-retrieve flow

**Non-Goals:**
- Full component library or design system extraction
- Animations or page transitions beyond toast enter/exit
- Accessibility audit (WCAG compliance is future work)
- Visual regression testing
- Storybook or component documentation

## Decisions

### 1. Sonner for toast notifications

Sonner is lightweight (~3KB), headless-friendly, works with Tailwind, requires minimal setup (one `<Toaster />` in the root), and has a simple imperative API (`toast.success("Copied!")`).

**Why not react-hot-toast?** Sonner has better default styling, built-in dark mode support, and is more actively maintained. Both are viable; sonner has a slightly smaller API surface.

### 2. Tailwind `dark:` variant with class strategy

Use Tailwind's `dark:` variant with `@custom-variant dark (&:where(.dark, .dark *))` in CSS. Theme toggle stores preference in `localStorage` and falls back to `prefers-color-scheme`. A toggle button in the Layout header switches between light/dark/system.

**Why class strategy over media-only?** Allows manual override while still respecting system preference as default.

### 3. Inline form validation without a schema library

Validate on blur and on submit. Simple validation functions (email regex, password length >= 8, required fields) — no Zod/Yup dependency. Field-level error messages shown below each input.

**Why not Zod?** The forms are simple (2-3 fields each). A schema library adds bundle size for minimal benefit here.

### 4. Spinner as a simple Tailwind component

A `<Spinner />` component using Tailwind's `animate-spin` on an SVG circle. No library needed. Used in buttons during async operations and as page-level loading indicators.

### 5. Playwright for e2e tests

Playwright tests run against the full Docker Compose stack. Test the core flow: create text secret → copy share link → open link → decrypt → verify plaintext. Also test file upload/download and auth flows.

**Why not Cypress?** Playwright has better multi-browser support, faster execution, and is the standard for modern React apps.

## Risks / Trade-offs

- **Dark mode requires touching every component** → Systematic pass using Tailwind `dark:` prefixes; most changes are mechanical (bg-gray-50 → dark:bg-gray-900, etc.)
- **Sonner adds a dependency** → Tiny (~3KB gzipped), well-maintained, no transitive deps of concern
- **E2e tests need running infrastructure** → Docker Compose provides the full stack; CI can run these in the build job
- **Form validation without a library may drift** → Acceptable for 3 simple forms; extract to shared validators if forms grow
