package domain

import (
	"time"

	"github.com/google/uuid"
)

type Secret struct {
	ID                 uuid.UUID  `json:"-"`
	PublicID           string     `json:"public_id"`
	RetrievalToken string `json:"-"`
	DeletionToken  string `json:"-"`
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
	PublicID          string `json:"public_id" validate:"required"`
	RetrievalToken    string `json:"retrieval_token" validate:"required"`
	DeletionToken     string `json:"deletion_token" validate:"required"`
	Nonce             string `json:"nonce" validate:"required"`
	EncryptedData     string `json:"encrypted_data" validate:"required,max=1048576"`
	Expiration        string `json:"expiration" validate:"required,expiration"`
	BurnAfterRead     bool   `json:"burn_after_read"`
	PasswordProtected bool   `json:"password_protected"`
}

type CreateFileRequest struct {
	PublicID          string `json:"public_id" validate:"required"`
	RetrievalToken    string `json:"retrieval_token" validate:"required"`
	DeletionToken     string `json:"deletion_token" validate:"required"`
	Nonce             string `json:"nonce" validate:"required"`
	Expiration        string `json:"expiration" validate:"required,expiration"`
	BurnAfterRead     bool   `json:"burn_after_read"`
	PasswordProtected bool   `json:"password_protected"`
	EncryptedFilename string `json:"encrypted_filename" validate:"required"`
}

type RetrieveSecretResponse struct {
	Nonce             string `json:"nonce"`
	EncryptedData     string `json:"encrypted_data"`
	SecretType        string `json:"secret_type"`
	BurnAfterRead     bool   `json:"burn_after_read"`
	PasswordProtected bool   `json:"password_protected"`
}

type SecretMetadataResponse struct {
	SecretType        string `json:"secret_type"`
	BurnAfterRead     bool   `json:"burn_after_read"`
	PasswordProtected bool   `json:"password_protected"`
	ExpiresAt         string `json:"expires_at"`
	CreatedAt         string `json:"created_at"`
	FileSize          *int64 `json:"file_size,omitempty"`
}
