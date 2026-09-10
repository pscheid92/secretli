# Secretli frontend

React + TypeScript single-page app, built with Vite and embedded into the Go binary at build time.

```bash
pnpm install
pnpm dev            # Vite dev server on :5173, proxies /api to the Go backend on :8080
pnpm test           # Vitest unit tests
pnpm lint           # Biome
pnpm build          # production build into dist/
pnpm e2e            # Playwright, needs the app running on :8080
pnpm e2e:large      # opt-in 1 GiB transfer test
```

All encryption happens in `src/lib/encryption.ts` and `src/lib/bundle.ts`; the server only ever sees ciphertext and token hashes. See the repository README for the overall architecture.
