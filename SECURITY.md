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

- All encryption and decryption happens client-side in the browser using [@noble/ciphers](https://github.com/paulmillr/noble-ciphers) and [@noble/hashes](https://github.com/paulmillr/noble-hashes)
- The server only stores opaque, encrypted blobs and never has access to plaintext data or encryption keys
- Encryption keys are transported via URL fragments (`#`), which are never sent to the server
- Metadata, blob, and deletion bearer tokens are stored as SHA-256 hashes, not raw tokens
- XChaCha20-Poly1305 is used for authenticated encryption with AAD binding per secret and purpose
- HKDF-SHA512 derives separate metadata keys, blob keys, public IDs, and access tokens from domain-separated labels
- Password protection uses scrypt (N=2^14, r=8, p=1) for blob key and blob token derivation

## Supported Versions

Only the latest release is supported with security updates.
