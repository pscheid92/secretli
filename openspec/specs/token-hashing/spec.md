## ADDED Requirements

### Requirement: SHA-256 token hashing
The server SHALL hash retrieval and deletion tokens using SHA-256 before storing them. The input is the raw base64url-decoded bytes of the token.

#### Scenario: Token is hashed deterministically
- **WHEN** the same token value is hashed twice
- **THEN** both hashes are identical

#### Scenario: Different tokens produce different hashes
- **WHEN** two different token values are hashed
- **THEN** the hashes are different

### Requirement: Constant-time token verification
The server SHALL compare provided tokens against stored hashes using `crypto/subtle.ConstantTimeCompare` to prevent timing attacks.

#### Scenario: Valid token passes verification
- **WHEN** a token is hashed and then verified against its own hash
- **THEN** verification succeeds

#### Scenario: Invalid token fails verification
- **WHEN** a different token is verified against a stored hash
- **THEN** verification fails

#### Scenario: Timing is constant regardless of input
- **WHEN** tokens are compared
- **THEN** the comparison uses `crypto/subtle.ConstantTimeCompare` (not `==` or `bytes.Equal`)
