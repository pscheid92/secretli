### Requirement: Periodic expired secret cleanup
The system SHALL run a background cleanup worker that periodically deletes expired secrets from the database and their associated S3 objects.

#### Scenario: Expired text secret is cleaned up
- **WHEN** the cleanup worker runs and a text secret has `expires_at < NOW()`
- **THEN** the secret row SHALL be deleted from the `secrets` table

#### Scenario: Expired file secret is cleaned up with S3 object
- **WHEN** the cleanup worker runs and a file secret has `expires_at < NOW()` and a non-null `storage_key`
- **THEN** the S3 object at `storage_key` SHALL be deleted AND the secret row SHALL be deleted from the database

#### Scenario: S3 deletion failure does not block database cleanup
- **WHEN** an S3 object deletion fails during cleanup
- **THEN** the error SHALL be logged and the worker SHALL continue processing remaining expired secrets

### Requirement: Configurable cleanup interval
The cleanup worker SHALL run at the interval specified by `CLEANUP_INTERVAL` (default: `1m`).

#### Scenario: Default cleanup interval
- **WHEN** `CLEANUP_INTERVAL` is not set
- **THEN** the cleanup worker SHALL run every 1 minute

#### Scenario: Custom cleanup interval
- **WHEN** `CLEANUP_INTERVAL` is set to `5m`
- **THEN** the cleanup worker SHALL run every 5 minutes

### Requirement: Graceful shutdown
The cleanup worker SHALL stop cleanly when the server receives a shutdown signal.

#### Scenario: Shutdown during idle
- **WHEN** the server receives SIGINT/SIGTERM and the cleanup worker is waiting for the next tick
- **THEN** the cleanup worker SHALL exit without starting a new cleanup cycle

#### Scenario: Shutdown during active cleanup
- **WHEN** the server receives SIGINT/SIGTERM and the cleanup worker is mid-cycle
- **THEN** the cleanup worker SHALL finish the current batch and then exit

### Requirement: Cleanup logging
The cleanup worker SHALL log the results of each cycle using structured logging.

#### Scenario: Secrets cleaned up
- **WHEN** expired secrets are deleted in a cycle
- **THEN** the worker SHALL log the count of deleted secrets and any S3 deletion errors at info level

#### Scenario: No expired secrets
- **WHEN** no expired secrets exist during a cycle
- **THEN** the worker SHALL NOT log (avoid log spam)
