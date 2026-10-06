package bundle

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"github.com/pscheid92/secretli/internal/share/keys"
)

// Encrypt builds the whole bundle in memory: every record, the manifest and
// the footer. Uploads stream records to the server instead (see the share
// package); this is for small bundles and for tests.
func Encrypt(plan *Plan, sources []Source, ks *keys.KeySet) ([]byte, error) {
	out := make([]byte, 0, plan.TotalSize)
	buf := make([]byte, ChunkSize)
	for _, record := range plan.Records {
		plaintext := buf[:record.PlaintextSize]
		if _, err := sources[record.FileIndex].Reader.ReadAt(plaintext, record.Start); err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("read %s: %w", sources[record.FileIndex].Name, err)
		}
		encrypted, err := ks.EncryptRecord(plaintext, ChunkAAD(record.FileIndex, record.ChunkIndex, record.PlaintextSize))
		if err != nil {
			return nil, err
		}
		if int64(len(encrypted)) != record.Length {
			return nil, errors.New("bundle record size mismatch")
		}
		out = append(out, encrypted...)
	}
	encryptedManifest, err := ks.EncryptRecord(plan.ManifestJSON, ManifestAAD())
	if err != nil {
		return nil, err
	}
	if int64(len(encryptedManifest)) != plan.EncryptedManifestLength {
		return nil, errors.New("bundle manifest size mismatch")
	}
	footer := Footer{ManifestLength: int64(len(encryptedManifest)), ManifestSHA256: sha256.Sum256(encryptedManifest)}
	trailer, err := EncodeFooter(footer)
	if err != nil {
		return nil, err
	}
	out = append(out, encryptedManifest...)
	out = append(out, trailer...)
	if int64(len(out)) != plan.TotalSize {
		return nil, errors.New("bundle size mismatch")
	}
	return out, nil
}
