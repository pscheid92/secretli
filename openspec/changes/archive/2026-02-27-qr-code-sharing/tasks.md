## 1. Dependency

- [x] 1.1 Install `qrcode` npm package and its TypeScript types (`@types/qrcode`) in `web/frontend/`

## 2. QR Code Component

- [x] 2.1 Create `web/frontend/src/components/QRCode.tsx` — accepts a `url: string` prop, generates a PNG data URL via `QRCode.toDataURL`, renders an `<img>` at 160×160 px with a white-background wrapper for dark mode compatibility
- [x] 2.2 Handle async generation (use `useEffect` + `useState` to store the data URL; render nothing or a placeholder until ready)

## 3. SecretResult Integration

- [x] 3.1 Import and render `<QRCode url={url} />` in `SecretResult.tsx`, positioned below the URL/copy row and above the expiry metadata
