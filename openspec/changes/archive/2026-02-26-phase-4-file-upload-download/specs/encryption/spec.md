## ADDED Requirements

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
