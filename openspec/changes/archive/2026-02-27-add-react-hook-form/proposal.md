## Why

Form state and validation in the React frontend is managed with manual `useState` hooks and inline if-checks scattered across `SecretForm`, `FilePage`, and `RetrievePage`. This leads to duplicated validation logic (both forms repeat expiration/password/burn-after-read patterns), no per-field error messages, and boilerplate for tracking dirty/touched state. Adopting `react-hook-form` centralizes form management, enables declarative validation with per-field errors, and reduces re-renders via uncontrolled inputs.

## What Changes

- Add `react-hook-form` as a frontend dependency
- Refactor `SecretForm.tsx` to use `useForm` — replace `useState` calls for form fields with `register`, add per-field validation rules and error display
- Refactor `FilePage.tsx` form to use `useForm` — same pattern for expiration, password, burn-after-read, and file selection
- Refactor the password form in `RetrievePage.tsx` to use `useForm` — replace manual password state/error handling
- Remove `src/lib/validation.ts` (unused utility, validation now handled by react-hook-form)
- Add per-field inline error messages below each input (e.g., "Secret text is required", "Password is required when protection is enabled")

## Capabilities

### New Capabilities

_None — this is a refactor of existing form handling, not a new feature._

### Modified Capabilities

- `secret-sharing-ui`: SecretForm and FilePage forms use `react-hook-form` for state management and validation; per-field error messages replace single generic error
- `file-encryption-ui`: FilePage form uses `react-hook-form` for non-file fields (expiration, password, burn-after-read); file selection stays as a controlled callback via `setValue`

## Impact

- **Dependencies**: New `react-hook-form` package in `web/frontend/package.json`
- **Components**: `SecretForm.tsx`, `FilePage.tsx`, `RetrievePage.tsx` (password form section)
- **Removed**: `src/lib/validation.ts`
- **No API changes**: Form data shape passed to encryption/API layer remains identical
- **No visual changes**: Same inputs, labels, and layout — only error messages become per-field
