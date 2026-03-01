## ADDED Requirements

### Requirement: Configuration from environment variables
The server SHALL load all configuration from environment variables as defined in DESIGN.md section 8. Each variable SHALL have a sensible default where applicable. Required variables without defaults SHALL cause the server to log a warning but not fail in Phase 1 (they become required in later phases when the features that use them are implemented).

#### Scenario: Default values used when env vars not set
- **WHEN** the server starts without any environment variables set
- **THEN** `SERVER_PORT` defaults to `8080`
- **AND** `S3_BUCKET` defaults to `secretli`
- **AND** `S3_USE_SSL` defaults to `true`
- **AND** `S3_REGION` defaults to `us-east-1`
- **AND** `MAX_FILE_SIZE` defaults to `104857600`
- **AND** `CLEANUP_INTERVAL` defaults to `1m`
- **AND** `SESSION_MAX_AGE` defaults to `720h`
- **AND** `COOKIE_SECURE` defaults to `true`

#### Scenario: Environment variables override defaults
- **WHEN** `SERVER_PORT` is set to `9090`
- **AND** `S3_BUCKET` is set to `my-bucket`
- **THEN** the config struct reflects these overridden values

### Requirement: Config struct with all fields
The `Config` struct SHALL include all fields from DESIGN.md section 8: `Port`, `DatabaseURL`, `S3Endpoint`, `S3Bucket`, `S3AccessKey`, `S3SecretKey`, `S3UseSSL`, `S3Region`, `MaxFileSize`, `CleanupInterval`, `SessionMaxAge`, `CookieDomain`, `CookieSecure`, `AllowedOrigins`.

#### Scenario: Config struct has all fields
- **WHEN** the config package is used
- **THEN** all fields listed in DESIGN.md section 8 are accessible on the `Config` struct with appropriate Go types (`string`, `bool`, `int64`, `time.Duration`)
