package domain

import "time"

type Secret struct {
	PublicID          string     `json:"public_id"`
	MetadataTokenHash string     `json:"-"`
	BlobTokenHash     string     `json:"-"`
	DeletionTokenHash string     `json:"-"`
	EncryptedMeta     string     `json:"encrypted_meta"`
	BlobSize          int64      `json:"blob_size"`
	BurnAfterRead     bool       `json:"burn_after_read"`
	ExpiresAt         time.Time  `json:"expires_at"`
	CreatedAt         time.Time  `json:"created_at"`
	RetrievedAt       *time.Time `json:"-"`
	StorageKey        string     `json:"-"`
}

type SecretMetadataResponse struct {
	EncryptedMeta string `json:"encrypted_meta"`
	BlobSize      int64  `json:"blob_size"`
	BurnAfterRead bool   `json:"burn_after_read"`
	ExpiresAt     string `json:"expires_at"`
	CreatedAt     string `json:"created_at"`
	// OpenedAt is when a recipient first opened a reusable secret, if one has.
	OpenedAt *string `json:"opened_at,omitempty"`
}

// TombstoneRetention is how long the server remembers what became of a
// secret after it is gone: a week past its expiry, or past its end if that
// came later.
const TombstoneRetention = 7 * 24 * time.Hour

// TombstoneKeepUntil is when the tombstone of a secret that expires at
// expiresAt and ended at endedAt is forgotten.
func TombstoneKeepUntil(expiresAt, endedAt time.Time) time.Time {
	if endedAt.After(expiresAt) {
		return endedAt.Add(TombstoneRetention)
	}
	return expiresAt.Add(TombstoneRetention)
}

// TombstoneOutcome is what became of a secret.
type TombstoneOutcome string

const (
	// TombstoneOpened: a one-time secret was read, which ended it.
	TombstoneOpened TombstoneOutcome = "opened"
	// TombstoneExpired: the secret reached its expiry.
	TombstoneExpired TombstoneOutcome = "expired"
	// TombstoneDeleted: the owner deleted it.
	TombstoneDeleted TombstoneOutcome = "deleted"
)

// SecretTombstone records what became of a secret after it is gone, so the
// links to it can say so. It holds no content and no key material.
type SecretTombstone struct {
	PublicID          string
	MetadataTokenHash string
	DeletionTokenHash string
	Outcome           TombstoneOutcome
	BurnAfterRead     bool
	// EndedAt is when the outcome happened; for an expired secret, its expiry.
	EndedAt time.Time
	// FirstOpenedAt is when a recipient first opened it, if anyone did. A
	// one-time secret was opened at EndedAt, unless the owner opened it.
	FirstOpenedAt *time.Time
	// OpenedByOwner marks a one-time secret the owner opened themselves, so
	// nobody else got it.
	OpenedByOwner bool
	KeepUntil     time.Time
}
