package model

import (
	"time"

	"github.com/google/uuid"
)

type Secret struct {
	ID                 uuid.UUID  `json:"-"`
	PublicID           string     `json:"public_id"`
	RetrievalTokenHash string     `json:"-"`
	DeletionTokenHash  string     `json:"-"`
	EncryptedData      *string    `json:"encrypted_data,omitempty"`
	Nonce              string     `json:"nonce"`
	SecretType         string     `json:"secret_type"`
	StorageKey         *string    `json:"-"`
	EncryptedFilename  *string    `json:"-"`
	EncryptedSize      *int64     `json:"-"`
	BurnAfterRead      bool       `json:"burn_after_read"`
	PasswordProtected  bool       `json:"password_protected"`
	ExpiresAt          time.Time  `json:"expires_at"`
	CreatedAt          time.Time  `json:"created_at"`
	RetrievedAt        *time.Time `json:"-"`
}

type CreateSecretRequest struct {
	PublicID          string `json:"public_id"`
	RetrievalToken    string `json:"retrieval_token"`
	DeletionToken     string `json:"deletion_token"`
	Nonce             string `json:"nonce"`
	EncryptedData     string `json:"encrypted_data"`
	Expiration        string `json:"expiration"`
	BurnAfterRead     bool   `json:"burn_after_read"`
	PasswordProtected bool   `json:"password_protected"`
}
