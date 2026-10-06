package keys

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func fixedSecret(t *testing.T) string {
	t.Helper()
	b := make([]byte, ShareSecretLength)
	for i := range b {
		b[i] = byte(i + 1)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func TestDerivationIsDeterministicAndShaped(t *testing.T) {
	a, err := FromShareSecret(fixedSecret(t), "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := FromShareSecret(fixedSecret(t), "")
	if err != nil {
		t.Fatal(err)
	}
	ea, eb := a.Encoded(), b.Encoded()
	if ea != eb {
		t.Fatalf("same secret derived differently: %+v vs %+v", ea, eb)
	}
	if len(ea.PublicID) != 22 || len(ea.MetadataToken) != 43 || len(ea.BlobToken) != 43 {
		t.Errorf("unexpected lengths: id %d, metadata %d, blob %d", len(ea.PublicID), len(ea.MetadataToken), len(ea.BlobToken))
	}
	if ea.DeletionToken != "" || a.HasDeletionToken() {
		t.Error("a key set from a link must not have a deletion token")
	}
	if ea.PublicID == ea.MetadataToken[:22] || ea.MetadataToken == ea.BlobToken {
		t.Error("derived values must differ from each other")
	}
}

func TestPasswordChangesOnlyTheBlobSide(t *testing.T) {
	plain, err := FromShareSecret(fixedSecret(t), "")
	if err != nil {
		t.Fatal(err)
	}
	withPassword, err := FromShareSecret(fixedSecret(t), "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	ep, ew := plain.Encoded(), withPassword.Encoded()
	if ep.PublicID != ew.PublicID || ep.MetadataToken != ew.MetadataToken {
		t.Error("the public id and metadata token must not depend on the password")
	}
	if ep.BlobToken == ew.BlobToken {
		t.Error("the blob token must depend on the password")
	}
	other, err := FromShareSecret(fixedSecret(t), "wrong horse")
	if err != nil {
		t.Fatal(err)
	}
	if other.Encoded().BlobToken == ew.BlobToken {
		t.Error("different passwords must give different blob tokens")
	}

	// Applying the password to a generated key set matches deriving with it.
	generated, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	applied, err := generated.WithPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	derived, err := FromShareSecret(generated.Encoded().ShareSecret, "pw")
	if err != nil {
		t.Fatal(err)
	}
	if applied.Encoded().BlobToken != derived.Encoded().BlobToken {
		t.Error("WithPassword and FromShareSecret with a password disagree")
	}
	if !applied.HasDeletionToken() || applied.Encoded().DeletionToken != generated.Encoded().DeletionToken {
		t.Error("WithPassword must keep the deletion token")
	}
}

func TestGenerateDrawsFreshSecrets(t *testing.T) {
	a, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if a.Encoded().ShareSecret == b.Encoded().ShareSecret || a.Encoded().DeletionToken == b.Encoded().DeletionToken {
		t.Error("two generated key sets share material")
	}
	if len(a.Encoded().ShareSecret) != 43 || len(a.Encoded().DeletionToken) != 43 {
		t.Errorf("unexpected lengths: secret %d, deletion %d", len(a.Encoded().ShareSecret), len(a.Encoded().DeletionToken))
	}
}

func TestRejectsMaterialOfTheWrongLength(t *testing.T) {
	if _, err := FromShareSecret(base64.RawURLEncoding.EncodeToString(make([]byte, 31)), ""); !errors.Is(err, ErrInvalidShareSecret) {
		t.Errorf("31 bytes: err = %v, want ErrInvalidShareSecret", err)
	}
	if _, err := FromShareSecret("not base64!", ""); !errors.Is(err, ErrInvalidShareSecret) {
		t.Errorf("garbage: err = %v, want ErrInvalidShareSecret", err)
	}
	ks, err := FromShareSecret(fixedSecret(t), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ks.WithDeletionToken("short"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("short deletion token: err = %v, want ErrInvalidToken", err)
	}
	owner, err := ks.WithDeletionToken(strings.Repeat("D", 43))
	if err != nil {
		t.Fatal(err)
	}
	if !owner.HasDeletionToken() || ks.HasDeletionToken() {
		t.Error("WithDeletionToken must return a copy with the token, leaving the original alone")
	}
}

func TestMetaEnvelopeRoundTrip(t *testing.T) {
	ks, err := FromShareSecret(fixedSecret(t), "")
	if err != nil {
		t.Fatal(err)
	}
	meta := Meta{Type: "bundle", PasswordProtected: true, BundleName: "Secretli bundle (2 files)"}
	envelope, err := ks.EncryptMeta(meta)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(envelope, "$")
	if len(parts) != 3 || parts[0] != "v2" {
		t.Fatalf("envelope = %q, want v2$nonce$ciphertext", envelope)
	}
	got, err := ks.DecryptMeta(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if got != meta {
		t.Errorf("meta = %+v, want %+v", got, meta)
	}

	other, err := FromShareSecret(base64.RawURLEncoding.EncodeToString(make([]byte, 32)), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.DecryptMeta(envelope); !errors.Is(err, ErrDecrypt) {
		t.Errorf("another key set: err = %v, want ErrDecrypt", err)
	}
	// Flip a bit of the ciphertext itself. Changing the last base64url
	// character would not do: unpadded base64 leaves it a few bits that
	// decoders ignore, so that change may decode to the same bytes.
	ciphertext, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	ciphertext[0] ^= 1
	tampered := parts[0] + "$" + parts[1] + "$" + base64.RawURLEncoding.EncodeToString(ciphertext)
	if _, err := ks.DecryptMeta(tampered); !errors.Is(err, ErrDecrypt) {
		t.Errorf("tampered ciphertext: err = %v, want ErrDecrypt", err)
	}
	if _, err := ks.DecryptMeta("v1$a$b"); !errors.Is(err, ErrInvalidEnvelope) {
		t.Errorf("old version: err = %v, want ErrInvalidEnvelope", err)
	}
	if _, err := ks.DecryptMeta("v2$short$" + parts[2]); !errors.Is(err, ErrInvalidEnvelope) {
		t.Errorf("short nonce: err = %v, want ErrInvalidEnvelope", err)
	}
}

func TestRecordsAreBoundToTheirPlace(t *testing.T) {
	ks, err := FromShareSecret(fixedSecret(t), "")
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("sixteen bytes!!!")
	record, err := ks.EncryptRecord(plaintext, []byte("chunk:0:0:16"))
	if err != nil {
		t.Fatal(err)
	}
	if len(record) != len(plaintext)+RecordOverhead {
		t.Fatalf("record length = %d, want %d", len(record), len(plaintext)+RecordOverhead)
	}
	got, err := ks.DecryptRecord(record, []byte("chunk:0:0:16"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("plaintext = %q, want %q", got, plaintext)
	}
	if _, err := ks.DecryptRecord(record, []byte("chunk:0:1:16")); !errors.Is(err, ErrDecrypt) {
		t.Errorf("moved record: err = %v, want ErrDecrypt", err)
	}
	record[len(record)-1] ^= 1
	if _, err := ks.DecryptRecord(record, []byte("chunk:0:0:16")); !errors.Is(err, ErrDecrypt) {
		t.Errorf("flipped bit: err = %v, want ErrDecrypt", err)
	}
	if _, err := ks.DecryptRecord(record[:RecordOverhead-1], []byte("chunk:0:0:16")); !errors.Is(err, ErrDecrypt) {
		t.Errorf("too short: err = %v, want ErrDecrypt", err)
	}

	// Two encryptions of the same record differ: the nonce is fresh each time.
	again, err := ks.EncryptRecord(plaintext, []byte("chunk:0:0:16"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(again[:NonceLength], record[:NonceLength]) {
		t.Error("nonce was reused")
	}
}
