package share_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/pscheid92/secretli/internal/share/bundle"
	"github.com/pscheid92/secretli/internal/share/keys"
)

// The TypeScript implementation (web/frontend/src/lib) and this one must read
// each other's output. Each side commits a vectors file the other side's
// tests decrypt: testdata/ts-vectors.json is written by
//
//	WRITE_VECTORS=1 pnpm vitest run src/lib/__tests__/vectors.test.ts
//
// in web/frontend, and testdata/go-vectors.json by
//
//	go test ./internal/share -run TestWritesGoVectors -args -write-vectors
var writeVectors = flag.Bool("write-vectors", false, "write testdata/go-vectors.json for the TypeScript tests")

type vectorFile struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	ContentBase64 string `json:"content_base64"`
}

type vectors struct {
	ShareSecret string `json:"share_secret"`
	Password    string `json:"password"`
	Derived     struct {
		PublicID          string `json:"public_id"`
		MetadataToken     string `json:"metadata_token"`
		BlobToken         string `json:"blob_token"`
		PasswordBlobToken string `json:"password_blob_token"`
	} `json:"derived"`
	Meta          keys.Meta    `json:"meta"`
	EncryptedMeta string       `json:"encrypted_meta"`
	Files         []vectorFile `json:"files"`
	BundleName    string       `json:"bundle_name"`
	// BundleBase64 is the whole encrypted bundle, sealed with the
	// password-derived blob keys.
	BundleBase64 string `json:"bundle_base64"`
}

func TestReadsTypeScriptVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "ts-vectors.json"))
	if err != nil {
		t.Fatalf("missing TypeScript vectors (see the comment at the top of this file): %v", err)
	}
	var v vectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}

	base, err := keys.FromShareSecret(v.ShareSecret, "")
	if err != nil {
		t.Fatal(err)
	}
	enc := base.Encoded()
	if enc.PublicID != v.Derived.PublicID || enc.MetadataToken != v.Derived.MetadataToken || enc.BlobToken != v.Derived.BlobToken {
		t.Errorf("derived %+v, TypeScript derived %+v", enc, v.Derived)
	}
	blob, err := keys.FromShareSecret(v.ShareSecret, v.Password)
	if err != nil {
		t.Fatal(err)
	}
	if blob.Encoded().BlobToken != v.Derived.PasswordBlobToken {
		t.Errorf("password blob token = %s, TypeScript %s", blob.Encoded().BlobToken, v.Derived.PasswordBlobToken)
	}

	meta, err := base.DecryptMeta(v.EncryptedMeta)
	if err != nil {
		t.Fatalf("decrypt TypeScript metadata: %v", err)
	}
	if meta != v.Meta {
		t.Errorf("meta = %+v, want %+v", meta, v.Meta)
	}

	data, err := base64.StdEncoding.DecodeString(v.BundleBase64)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	fetch := func(_ context.Context, start, end int64) ([]byte, error) { return data[start : end+1], nil }
	manifest, err := bundle.ReadManifest(ctx, fetch, blob, int64(len(data)))
	if err != nil {
		t.Fatalf("read TypeScript bundle: %v", err)
	}
	if manifest.BundleName != v.BundleName || len(manifest.Files) != len(v.Files) {
		t.Fatalf("manifest = %+v", manifest)
	}
	for i, want := range v.Files {
		wantContent, err := base64.StdEncoding.DecodeString(want.ContentBase64)
		if err != nil {
			t.Fatal(err)
		}
		file := manifest.Files[i]
		if file.Name != want.Name || file.Type != want.Type {
			t.Errorf("file %d = %s (%s), want %s (%s)", i, file.Name, file.Type, want.Name, want.Type)
		}
		var out bytes.Buffer
		if err := bundle.DecryptFile(ctx, fetch, blob, file, &out, nil); err != nil {
			t.Fatalf("decrypt %s: %v", want.Name, err)
		}
		if !bytes.Equal(out.Bytes(), wantContent) {
			t.Errorf("%s: got %d bytes, want %d", want.Name, out.Len(), len(wantContent))
		}
	}
}

func TestWritesGoVectors(t *testing.T) {
	if !*writeVectors {
		t.Skip("run with -args -write-vectors to regenerate testdata/go-vectors.json")
	}
	secret := make([]byte, keys.ShareSecretLength)
	for i := range secret {
		secret[i] = byte(0x80 + i)
	}
	var v vectors
	v.ShareSecret = base64.RawURLEncoding.EncodeToString(secret)
	v.Password = "correct horse battery staple"

	base, err := keys.FromShareSecret(v.ShareSecret, "")
	if err != nil {
		t.Fatal(err)
	}
	blob, err := keys.FromShareSecret(v.ShareSecret, v.Password)
	if err != nil {
		t.Fatal(err)
	}
	enc := base.Encoded()
	v.Derived.PublicID, v.Derived.MetadataToken, v.Derived.BlobToken = enc.PublicID, enc.MetadataToken, enc.BlobToken
	v.Derived.PasswordBlobToken = blob.Encoded().BlobToken

	contents := [][]byte{[]byte("hello from go\n"), {}, make([]byte, 300)}
	for i := range contents[2] {
		contents[2][i] = byte(255 - i%256)
	}
	v.Files = []vectorFile{
		{Name: "hello.txt", Type: "text/plain", ContentBase64: base64.StdEncoding.EncodeToString(contents[0])},
		{Name: "empty.bin", Type: "application/octet-stream", ContentBase64: ""},
		{Name: "bytes.bin", Type: "application/octet-stream", ContentBase64: base64.StdEncoding.EncodeToString(contents[2])},
	}
	sources := make([]bundle.Source, len(v.Files))
	names := make([]string, len(v.Files))
	for i, f := range v.Files {
		sources[i] = bundle.Source{Name: f.Name, Type: f.Type, Size: int64(len(contents[i])), Reader: bytes.NewReader(contents[i])}
		names[i] = f.Name
	}
	v.BundleName = bundle.DefaultBundleName(names)
	v.Meta = keys.Meta{Type: "bundle", PasswordProtected: true, BundleName: v.BundleName}
	if v.EncryptedMeta, err = base.EncryptMeta(v.Meta); err != nil {
		t.Fatal(err)
	}
	plan, err := bundle.NewPlan(sources, v.BundleName)
	if err != nil {
		t.Fatal(err)
	}
	data, err := bundle.Encrypt(plan, sources, blob)
	if err != nil {
		t.Fatal(err)
	}
	v.BundleBase64 = base64.StdEncoding.EncodeToString(data)

	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("testdata", "go-vectors.json"), append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
