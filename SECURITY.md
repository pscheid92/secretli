# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Secretli, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please email **patrick@pscheid.dev** with:

- A description of the vulnerability
- Steps to reproduce the issue
- Any potential impact you've identified

You should receive a response within 48 hours. If the issue is confirmed, a fix will be developed and released as soon as possible.

## Security Model

Secretli uses a zero-knowledge architecture:

- All encryption and decryption happens client-side in the browser using the Web Crypto API
- The server only stores opaque, encrypted blobs and never has access to plaintext data or encryption keys
- Encryption keys are transported via URL fragments (`#`), which are never sent to the server
- AES-256-GCM is used for encryption with unique nonces per operation
- HKDF-SHA512 derives separate encryption keys, public IDs, and retrieval tokens from a single master secret
- Password protection uses PBKDF2-SHA512 with 210,000 iterations

## Supported Versions

Only the latest release is supported with security updates.
