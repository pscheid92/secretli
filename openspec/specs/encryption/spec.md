## ADDED Requirements

### Requirement: URL-safe base64 encoding
The `lib/base64.ts` module SHALL encode `Uint8Array` to URL-safe base64 strings with no padding, and decode back. It MUST match Go's `base64.RawURLEncoding` output exactly (characters `+`→`-`, `/`→`_`, no trailing `=`).

#### Scenario: Encode bytes to URL-safe base64
- **WHEN** encoding a `Uint8Array` with `base64UrlEncode()`
- **THEN** the output string uses `-` and `_` instead of `+` and `/`, and has no `=` padding

#### Scenario: Decode URL-safe base64 to bytes
- **WHEN** decoding a URL-safe base64 string with `base64UrlDecode()`
- **THEN** the output `Uint8Array` matches the original input bytes

#### Scenario: Round-trip compatibility with Go
- **WHEN** Go encodes bytes with `base64.RawURLEncoding.EncodeToString()` and TypeScript decodes with `base64UrlDecode()`
- **THEN** the decoded bytes are identical to the Go input bytes

### Requirement: HKDF-SHA512 key derivation
The `KeySet` class SHALL derive three values from a 32-byte share secret using HKDF-SHA512 with no salt: an encryption key (32 bytes, info `"share_item_encryption_key"`), a public ID (16 bytes, info `"share_item_uuid"`), and a retrieval token (16 bytes, info `"share_item_token"`).

#### Scenario: Derive keys from share secret
- **WHEN** `KeySet.fromShareSecret(shareSecretBytes)` is called
- **THEN** the returned KeySet contains a 32-byte encryption key, 16-byte public ID, and 16-byte retrieval token, all derived via HKDF-SHA512

#### Scenario: Deterministic derivation
- **WHEN** the same 32-byte share secret is used twice
- **THEN** the derived encryption key, public ID, and retrieval token are identical both times

### Requirement: Random deletion token
The `KeySet` SHALL generate a 16-byte cryptographically random deletion token that is independent of the share secret.

#### Scenario: Deletion token is random
- **WHEN** a new `KeySet` is created
- **THEN** the deletion token is 16 bytes of cryptographically random data, not derived from HKDF

### Requirement: AES-256-GCM encryption
The `KeySet.encrypt(plaintext)` method SHALL encrypt UTF-8 text using AES-256-GCM with the derived encryption key and a random 12-byte nonce. It MUST return the nonce and ciphertext as URL-safe base64 strings.

#### Scenario: Encrypt plaintext text
- **WHEN** `keySet.encrypt("hello world")` is called
- **THEN** it returns `{ nonce: string, encrypted_data: string }` where both values are URL-safe base64 encoded

#### Scenario: Different nonce each time
- **WHEN** the same plaintext is encrypted twice with the same KeySet
- **THEN** the nonce values differ (random 12-byte IV each time)

### Requirement: AES-256-GCM decryption
The `KeySet.decrypt(nonce, encryptedData)` method SHALL decrypt AES-256-GCM ciphertext using the derived encryption key, returning the original UTF-8 plaintext.

#### Scenario: Round-trip encrypt then decrypt
- **WHEN** plaintext is encrypted with `keySet.encrypt()` and then decrypted with `keySet.decrypt()`
- **THEN** the decrypted text matches the original plaintext exactly

#### Scenario: Decryption with wrong key fails
- **WHEN** ciphertext encrypted by one KeySet is decrypted by a different KeySet
- **THEN** decryption throws an error

### Requirement: PBKDF2 password support
When a password is provided, the `KeySet` SHALL derive a 32-byte master key via `PBKDF2-SHA512(password, share_secret_bytes, 100000, 32)` and use that master key as input to HKDF instead of the raw share secret. The original share secret MUST be preserved unchanged for the URL.

#### Scenario: Create KeySet with password
- **WHEN** `KeySet.fromShareSecret(shareSecretBytes, password)` is called with a non-empty password
- **THEN** PBKDF2-SHA512 derives a master key from the password and share secret, and HKDF uses that master key

#### Scenario: Decrypt requires same password
- **WHEN** a secret is encrypted with a password and decryption is attempted without the password
- **THEN** decryption fails (wrong derived keys)

#### Scenario: Same share secret, different passwords produce different keys
- **WHEN** the same share secret is used with different passwords
- **THEN** the derived encryption key, public ID, and retrieval token are all different

### Requirement: New random KeySet generation
`KeySet.generateRandom()` SHALL generate a fresh 32-byte cryptographically random share secret and derive all keys from it.

#### Scenario: Generate new KeySet
- **WHEN** `KeySet.generateRandom()` is called
- **THEN** a new KeySet is returned with a random share secret and all derived keys

### Requirement: KeySet from existing share secret
`KeySet.fromShareSecret(encoded)` SHALL reconstruct a KeySet from a base64url-encoded share secret string (as found in the URL fragment).

#### Scenario: Reconstruct KeySet from share URL
- **WHEN** `KeySet.fromShareSecret(base64UrlEncodedSecret)` is called
- **THEN** the same public ID, retrieval token, and encryption key are derived as when the KeySet was originally created

### Requirement: File encryption
The `KeySet` class SHALL provide an `encryptFile(data: Uint8Array)` method that encrypts binary data using AES-256-GCM with the derived encryption key and a random 12-byte nonce. It SHALL return `{ nonce: string, encryptedBlob: Blob }` where the nonce is base64url-encoded and the encrypted data is a raw binary Blob (not base64-encoded).

#### Scenario: Encrypt a file
- **WHEN** `keySet.encryptFile(fileBytes)` is called with a Uint8Array
- **THEN** it returns a base64url nonce and a Blob containing the AES-256-GCM ciphertext

#### Scenario: Different nonce each encryption
- **WHEN** the same file data is encrypted twice with the same KeySet
- **THEN** the nonce values differ (random 12-byte IV each time)

### Requirement: File decryption
The `KeySet` class SHALL provide a `decryptFile(nonce: string, encryptedBlob: Blob)` method that decrypts the binary Blob using AES-256-GCM with the derived encryption key. It SHALL return the original `Uint8Array`.

#### Scenario: Round-trip file encrypt then decrypt
- **WHEN** a file is encrypted with `encryptFile()` and then decrypted with `decryptFile()`
- **THEN** the decrypted bytes match the original file bytes exactly

#### Scenario: Decryption with wrong key fails
- **WHEN** an encrypted file is decrypted with a different KeySet
- **THEN** decryption throws an error

### Requirement: Filename encryption
The `KeySet` class SHALL provide an `encryptFilename(filename: string)` method that encrypts the filename as a UTF-8 string using AES-256-GCM and returns a single string combining the base64url nonce and ciphertext (format: `nonce:ciphertext`). A corresponding `decryptFilename(encrypted: string)` method SHALL split and decrypt.

#### Scenario: Encrypt and decrypt filename
- **WHEN** a filename is encrypted with `encryptFilename("document.pdf")` and decrypted with `decryptFilename()`
- **THEN** the decrypted filename matches `"document.pdf"` exactly

#### Scenario: Encrypted filename format
- **WHEN** `encryptFilename` is called
- **THEN** the returned string is in the format `<base64url_nonce>:<base64url_ciphertext>`
