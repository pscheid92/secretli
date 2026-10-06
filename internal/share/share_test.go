package share_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/pscheid92/secretli/internal/share"
	"github.com/pscheid92/secretli/internal/share/api"
	"github.com/pscheid92/secretli/internal/share/bundle"
	"github.com/pscheid92/secretli/internal/share/sharetest"
)

// memorySink keeps what Open hands over.
type memorySink struct {
	text  []byte
	files map[string]*bytes.Buffer
}

func (m *memorySink) Text(text []byte) error { m.text = text; return nil }

func (m *memorySink) File(file bundle.File) (io.WriteCloser, error) {
	if m.files == nil {
		m.files = map[string]*bytes.Buffer{}
	}
	buf := &bytes.Buffer{}
	m.files[file.Name] = buf
	return nopCloser{buf}, nil
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

func newClient(t *testing.T, partSize int64) (*api.Client, *sharetest.Server) {
	t.Helper()
	srv := sharetest.New(partSize)
	t.Cleanup(srv.Close)
	return api.New(srv.URL), srv
}

func TestShareAndOpenText(t *testing.T) {
	ctx := context.Background()
	c, _ := newClient(t, 32*1024*1024)

	result, err := share.Share(ctx, c, share.Params{Text: []byte("the launch code is 0000")})
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != share.KindText || result.Reusable || result.PasswordProtected || result.Size != 23 {
		t.Errorf("result = %+v", result)
	}
	if !result.Link.IsOwner() || !strings.HasPrefix(result.Link.String(), c.BaseURL+"/s#") {
		t.Errorf("link = %s", result.Link)
	}
	recipient := result.Link.Recipient()
	if recipient.IsOwner() || !strings.Contains(result.Link.String(), recipient.String()+"!") {
		t.Errorf("recipient link = %s, owner link = %s", recipient, result.Link)
	}

	info, err := share.Inspect(ctx, c, recipient)
	if err != nil {
		t.Fatal(err)
	}
	if info.Kind != share.KindText || info.PasswordProtected || info.Reusable || info.BundleName != "secret.txt" {
		t.Errorf("info = %+v", info)
	}

	sink := &memorySink{}
	opened, err := share.Open(ctx, c, recipient, "", sink, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(sink.text) != "the launch code is 0000" || opened.Info.Kind != share.KindText {
		t.Errorf("text = %q", sink.text)
	}

	// A one-time secret is gone once opened, and its link says so.
	_, err = share.Inspect(ctx, c, recipient)
	var gone *share.GoneError
	if !errors.As(err, &gone) || gone.Gone.Outcome != "opened" || gone.Gone.OpenedByOwner {
		t.Fatalf("after opening: err = %v, want a GoneError with outcome opened", err)
	}
	if _, err := share.Open(ctx, c, recipient, "", &memorySink{}, nil); !errors.As(err, &gone) {
		t.Errorf("second open: err = %v, want GoneError", err)
	}
}

func TestShareFilesWithPasswordInSeveralParts(t *testing.T) {
	ctx := context.Background()
	// A 9 MiB part size with 4 MiB records makes 8 MiB parts, so a 20 MiB file
	// goes up as three parts, two of them at once.
	c, srv := newClient(t, 9*1024*1024)
	big := make([]byte, 20*1024*1024)
	for i := range big {
		big[i] = byte(i * 7)
	}
	files := []bundle.Source{
		{Name: "big.bin", Size: int64(len(big)), Reader: bytes.NewReader(big)},
		{Name: "notes.txt", Type: "text/plain", Size: 5, Reader: strings.NewReader("notes")},
	}
	var progressCalls int
	var lastUploaded, lastTotal int64
	result, err := share.Share(ctx, c, share.Params{
		Files: files, Password: "hunter2", Reusable: true, Expiration: "1h",
		Progress: func(uploaded, total int64) { progressCalls++; lastUploaded, lastTotal = uploaded, total },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != share.KindFiles || !result.Reusable || !result.PasswordProtected || len(result.Names) != 2 {
		t.Errorf("result = %+v", result)
	}
	if progressCalls < 2 || lastUploaded != lastTotal {
		t.Errorf("progress: %d calls, ended at %d of %d", progressCalls, lastUploaded, lastTotal)
	}
	if parts := srv.PartsUploaded(); parts != 3 {
		t.Errorf("parts uploaded = %d, want 3", parts)
	}

	recipient := result.Link.Recipient()
	info, err := share.Inspect(ctx, c, recipient)
	if err != nil {
		t.Fatal(err)
	}
	if !info.PasswordProtected || !info.Reusable || info.Kind != share.KindFiles || info.BundleName != "Secretli bundle (2 files)" {
		t.Errorf("info = %+v", info)
	}
	if _, err := share.Open(ctx, c, recipient, "", &memorySink{}, nil); !errors.Is(err, share.ErrPasswordRequired) {
		t.Errorf("no password: err = %v", err)
	}
	if _, err := share.Open(ctx, c, recipient, "wrong", &memorySink{}, nil); !errors.Is(err, share.ErrWrongPassword) {
		t.Errorf("wrong password: err = %v", err)
	}

	sink := &memorySink{}
	var last int64
	if _, err := share.Open(ctx, c, recipient, "hunter2", sink, func(done, _ int64) { last = done }); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sink.files["big.bin"].Bytes(), big) || sink.files["notes.txt"].String() != "notes" {
		t.Error("decrypted files differ from the originals")
	}
	if last != int64(len(big))+5 {
		t.Errorf("progress ended at %d", last)
	}

	// Reusable: the owner's own look is not an opening, a recipient's is.
	if _, err := share.Open(ctx, c, result.Link, "hunter2", &memorySink{}, nil); err != nil {
		t.Fatal(err)
	}
	info, err = share.Inspect(ctx, c, result.Link)
	if err != nil {
		t.Fatal(err)
	}
	if info.OpenedAt == nil {
		t.Error("a recipient opened it, so opened_at must be set")
	}
}

func TestDeleteNeedsTheOwnerLinkAndLeavesAStory(t *testing.T) {
	ctx := context.Background()
	c, srv := newClient(t, 32*1024*1024)
	result, err := share.Share(ctx, c, share.Params{Text: []byte("bye")})
	if err != nil {
		t.Fatal(err)
	}
	if err := share.Delete(ctx, c, result.Link.Recipient()); !errors.Is(err, share.ErrNotOwner) {
		t.Errorf("recipient link: err = %v, want ErrNotOwner", err)
	}
	if err := share.Delete(ctx, c, result.Link); err != nil {
		t.Fatal(err)
	}
	if srv.Secrets() != 0 {
		t.Error("the secret is still on the server")
	}
	_, err = share.Inspect(ctx, c, result.Link)
	var gone *share.GoneError
	if !errors.As(err, &gone) || gone.Gone.Outcome != "deleted" {
		t.Errorf("after delete: err = %v, want GoneError deleted", err)
	}
}

func TestLinks(t *testing.T) {
	secret := strings.Repeat("a", 43)
	token := strings.Repeat("b", 43)
	link, err := share.ParseLink(" https://secretli.app/s#" + secret + "!" + token + " ")
	if err != nil {
		t.Fatal(err)
	}
	if link.Origin != "https://secretli.app" || link.Secret != secret || link.DeletionToken != token || !link.IsOwner() {
		t.Errorf("link = %+v", link)
	}
	if link.Recipient().String() != "https://secretli.app/s#"+secret {
		t.Errorf("recipient = %s", link.Recipient())
	}
	for _, bad := range []string{"", "secretli.app/s#" + secret, "https://secretli.app/share#" + secret, "https://secretli.app/s#" + secret[:42], "https://secretli.app/s#" + secret + "!short", "ftp://x/s#" + secret} {
		if _, err := share.ParseLink(bad); !errors.Is(err, share.ErrNotALink) {
			t.Errorf("%q: err = %v, want ErrNotALink", bad, err)
		}
	}
	if _, err := share.Share(context.Background(), api.New("http://127.0.0.1:1"), share.Params{Text: []byte("x"), Expiration: "2h"}); err == nil || !strings.Contains(err.Error(), "5m, 10m") {
		t.Errorf("bad expiry: err = %v", err)
	}
	if _, err := share.Share(context.Background(), api.New("http://127.0.0.1:1"), share.Params{}); !errors.Is(err, share.ErrNothingToShare) {
		t.Errorf("nothing: err = %v", err)
	}
}
