## Context

The frontend has 3 forms: text secret creation (`SecretForm.tsx`), file upload (`FilePage.tsx`), and password entry (`RetrievePage.tsx`). All use `useState` for each field plus manual validation in `handleSubmit`. The text and file forms share nearly identical controls (expiration picker, burn-after-read checkbox, optional password). Error handling is a single `error` string state displayed below the textarea/form, not tied to specific fields.

## Goals / Non-Goals

**Goals:**
- Replace `useState`-based form state with `react-hook-form`'s `useForm` in all 3 forms
- Add per-field validation with inline error messages
- Remove the unused `src/lib/validation.ts` file
- Keep the same visual layout and form behavior

**Non-Goals:**
- Adding Zod schema validation (react-hook-form's built-in `register` rules are sufficient for these simple forms)
- Refactoring the `FileUpload` drag-and-drop component itself (it remains a controlled callback; `useForm`'s `setValue` bridges it)
- Changing the `ExpirationPicker` component API (it continues to receive `value`/`onChange`, wired via `Controller` or `register`)
- Adding new form fields or changing the data submitted to the API

## Decisions

### Use `register` rules over Zod resolver

The forms have simple validation: required fields, conditional required (password when toggle is on). `register("text", { required: "Secret text is required" })` is simpler than introducing a Zod schema + resolver for this complexity level.

**Alternative**: Zod resolver — rejected as over-engineered. Can be added later if validation grows more complex.

### Use `Controller` for `ExpirationPicker` and `FileUpload`

`ExpirationPicker` is a custom `<select>` wrapper that takes `value`/`onChange` — it needs `Controller` to integrate with react-hook-form. Similarly, `FileUpload` calls back via `onSelect(files)` — use `setValue("files", files)` in the callback to sync with form state.

**Alternative**: Convert `ExpirationPicker` to a native `<select>` with `register` — rejected because it changes the component's API unnecessarily.

### Keep `onSubmit` callback pattern for SecretForm

`SecretForm` currently receives an `onSubmit` prop from `SharePage`. This stays the same — `handleSubmit` from react-hook-form calls the existing `onSubmit(data)` prop. The form data shape (`SecretFormData`) doesn't change.

### Password field: conditional validation via `validate` function

The password field is only required when `showPassword` is true. Use react-hook-form's `validate` option: `register("password", { validate: (v) => !showPassword || v.length > 0 || "Password is required" })`. The `showPassword` state stays as a local `useState` since it controls UI visibility, not form data.

## Risks / Trade-offs

- **[Bundle size]** → `react-hook-form` is ~9KB gzipped. Acceptable for the functionality it provides.
- **[Learning curve for contributors]** → react-hook-form is the most popular React form library. Low risk.
- **[File input bridging]** → `FileUpload` uses a callback pattern that doesn't map directly to `register`. Using `setValue` is a well-documented pattern for custom inputs.
