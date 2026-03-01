## Why

The application is functionally complete and deployable but lacks the UX polish expected of a production product. Error feedback is inline-only with no toast notifications, forms use minimal HTML5 validation, there's no dark mode, loading states are text-only, and there are zero frontend tests. This phase brings the UI to production quality.

## What Changes

- **Toast notifications**: Add a lightweight toast system (sonner) for success/error feedback across all user actions (copy, create, delete, auth)
- **Loading states**: Replace text-only loading messages with spinner components and skeleton placeholders
- **Form validation**: Add real-time field-level validation to auth forms (email format, password strength, required fields) and secret creation forms
- **Copy-to-clipboard enhancement**: Add copy buttons on retrieved secret text, toast confirmation on copy
- **Dark mode**: Add system-preference-aware dark/light theme with manual toggle, using Tailwind's `dark:` variant
- **Responsive design**: Improve mobile nav (hamburger menu), convert history table to cards on small screens
- **Playwright e2e tests**: End-to-end tests for the core share-and-retrieve flow

## Capabilities

### New Capabilities
- `toast-notifications`: Global toast notification system for success, error, and info feedback
- `dark-mode`: System-preference-aware dark/light theme with manual toggle
- `e2e-tests`: Playwright end-to-end tests for core user flows

### Modified Capabilities
- `secret-sharing-ui`: Add copy button on retrieved text, toast on copy, loading spinner, form validation
- `file-encryption-ui`: Add loading spinner for upload/download, toast on success/error
- `user-auth`: Add field-level validation to login/register forms, loading spinners
- `react-app`: Add responsive mobile nav, improve mobile layout for history page

## Impact

- **New dependencies**: `sonner` (toast library), `@playwright/test` (dev dependency)
- **Modified files**: All page components, Layout.tsx, SecretForm.tsx, SecretResult.tsx, FileUpload.tsx, index.css
- **New files**: Toast provider component, Spinner component, Playwright config + test files
- **No backend changes**: This is purely a frontend phase
