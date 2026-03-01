## 1. Setup

- [x] 1.1 Install `react-hook-form` in `web/frontend/`

## 2. SecretForm refactor

- [x] 2.1 Replace `useState` calls for form fields (`text`, `expiration`, `burnAfterRead`, `showPassword`, `password`) with `useForm` in `SecretForm.tsx`
- [x] 2.2 Wire `ExpirationPicker` via `Controller` and textarea/password via `register` with validation rules
- [x] 2.3 Add per-field inline error messages below textarea and password input
- [x] 2.4 Wire `handleSubmit` from react-hook-form to call the existing `onSubmit` prop with the same data shape

## 3. FilePage refactor

- [x] 3.1 Replace `useState` calls for form fields in `FilePage.tsx` with `useForm`
- [x] 3.2 Bridge `FileUpload` selection to form state via `setValue("files", ...)` and add file required validation
- [x] 3.3 Wire `ExpirationPicker` via `Controller` and password via `register` with conditional validation
- [x] 3.4 Add per-field inline error messages below file upload area and password input

## 4. RetrievePage password form refactor

- [x] 4.1 Replace `useState` for password/passwordError in the password entry section of `RetrievePage.tsx` with `useForm`
- [x] 4.2 Add per-field inline error message for the password input

## 5. Cleanup

- [x] 5.1 Remove `src/lib/validation.ts`
- [x] 5.2 Verify the app builds with no TypeScript errors (`npm run build`)
