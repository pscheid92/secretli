// Package share does what the web app does, without the browser: it makes a
// secret from text or files and uploads it, and it opens, inspects and
// deletes secrets by their links. The server never sees anything but
// ciphertext and token hashes, exactly as with the browser.
package share

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/pscheid92/secretli/internal/share/api"
	"github.com/pscheid92/secretli/internal/share/bundle"
	"github.com/pscheid92/secretli/internal/share/keys"
)

const (
	// MaxEncryptedUploadBytes is the server's default upload limit.
	MaxEncryptedUploadBytes = 1024 * 1024 * 1024
	// MinPartBytes is S3's minimum for every part but the last.
	MinPartBytes = 5 * 1024 * 1024
	// UploadConcurrency is how many parts are in flight at once.
	UploadConcurrency = 3

	textFileName = "secret.txt"
)

// Expirations are the lifetimes the server accepts, shortest first.
var Expirations = []string{"5m", "10m", "15m", "1h", "4h", "12h", "1d", "3d", "7d"}

// DefaultExpiration is what the web app picks.
const DefaultExpiration = "1d"

// ValidExpiration reports whether the server accepts this lifetime.
func ValidExpiration(s string) bool {
	return slices.Contains(Expirations, s)
}

var (
	// ErrNotALink is text that is not a Secretli link.
	ErrNotALink = errors.New("not a Secretli link")
	// ErrNotOwner is an operation that needs the owner link's deletion token.
	ErrNotOwner = errors.New("this needs the owner link, the one with the part after \"!\"")
	// ErrPasswordRequired is a secret with a password when none was given.
	ErrPasswordRequired = errors.New("this secret needs a password")
	// ErrWrongPassword is a password the server's blob token did not match.
	ErrWrongPassword = errors.New("wrong password")
	// ErrLinkMismatch is a link whose tokens do not fit the secret they name.
	ErrLinkMismatch = errors.New("this link doesn't fit the secret it points to")
	// ErrTooLarge is a share beyond the upload limit.
	ErrTooLarge = errors.New("too large to share: the limit is 1 GiB")
	// ErrNothingToShare is a share with neither text nor files.
	ErrNothingToShare = errors.New("nothing to share")
)

// GoneError is a secret that is no longer there, with what became of it.
type GoneError struct {
	Gone api.Gone
}

func (e *GoneError) Error() string {
	return "this secret is gone: " + e.Gone.Outcome
}

// NotFoundError is a secret the server has no record of, not even a tombstone.
type NotFoundError struct{}

func (*NotFoundError) Error() string {
	return "this secret is gone: it was opened already, or it expired"
}

var linkFragment = regexp.MustCompile(`^([A-Za-z0-9_-]{43})(?:!([A-Za-z0-9_-]{43}))?$`)

// Link is a share link: the server, the share secret and, for the owner
// link, the deletion token.
type Link struct {
	Origin        string
	Secret        string
	DeletionToken string
}

// ParseLink reads a link as the web app prints it: https://host/s#secret or
// https://host/s#secret!deletionToken.
func ParseLink(raw string) (Link, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.Path != "/s" {
		return Link{}, ErrNotALink
	}
	m := linkFragment.FindStringSubmatch(u.Fragment)
	if m == nil {
		return Link{}, ErrNotALink
	}
	return Link{Origin: u.Scheme + "://" + u.Host, Secret: m[1], DeletionToken: m[2]}, nil
}

// IsOwner reports whether the link carries the deletion token.
func (l Link) IsOwner() bool { return l.DeletionToken != "" }

// Recipient is the link without the deletion token, the one to hand out.
func (l Link) Recipient() Link { return Link{Origin: l.Origin, Secret: l.Secret} }

func (l Link) String() string {
	s := l.Origin + "/s#" + l.Secret
	if l.DeletionToken != "" {
		s += "!" + l.DeletionToken
	}
	return s
}

// Kind is what a secret holds.
type Kind string

const (
	// KindText is a note, shared as a single secret.txt inside the bundle.
	KindText Kind = "text"
	// KindFiles is one or more files.
	KindFiles Kind = "files"
)

// Params describes a secret to make.
type Params struct {
	// Files are shared as a bundle; when empty, Text is shared instead.
	Files []bundle.Source
	Text  []byte
	// Expiration is one of Expirations; empty means DefaultExpiration.
	Expiration string
	// Reusable keeps the link working until it expires; the default is once.
	Reusable bool
	Password string
	// Progress, if set, is told about uploaded bytes.
	Progress func(uploaded, total int64)
}

// Result is a shared secret: its links and what the server confirmed.
type Result struct {
	Link              Link
	ExpiresAt         time.Time
	Kind              Kind
	Size              int64
	Names             []string
	Reusable          bool
	PasswordProtected bool
}

// Share encrypts and uploads a secret and returns its links.
func Share(ctx context.Context, c *api.Client, p Params) (*Result, error) {
	expiration := p.Expiration
	if expiration == "" {
		expiration = DefaultExpiration
	}
	if !ValidExpiration(expiration) {
		return nil, fmt.Errorf("expiry %q is not one of %s", expiration, strings.Join(Expirations, ", "))
	}

	kind := KindFiles
	sources := p.Files
	if len(sources) == 0 {
		if len(p.Text) == 0 {
			return nil, ErrNothingToShare
		}
		kind = KindText
		sources = []bundle.Source{{Name: textFileName, Type: "text/plain", Size: int64(len(p.Text)), Reader: bytes.NewReader(p.Text)}}
	}
	names := make([]string, 0, len(sources))
	sizes := make([]int64, 0, len(sources))
	var total int64
	for _, s := range sources {
		names = append(names, s.Name)
		sizes = append(sizes, s.Size)
		total += s.Size
	}
	if bundle.EstimateEncryptedSize(sizes) > MaxEncryptedUploadBytes {
		return nil, ErrTooLarge
	}

	base, err := keys.Generate()
	if err != nil {
		return nil, err
	}
	blobKeys, err := base.WithPassword(p.Password)
	if err != nil {
		return nil, err
	}
	bundleName := bundle.DefaultBundleName(names)
	plan, err := bundle.NewPlan(sources, bundleName)
	if err != nil {
		return nil, err
	}
	metaType := "bundle"
	if kind == KindText {
		metaType = "text"
	}
	encryptedMeta, err := base.EncryptMeta(keys.Meta{Type: metaType, PasswordProtected: p.Password != "", BundleName: bundleName})
	if err != nil {
		return nil, err
	}

	enc := base.Encoded()
	session, err := c.StartUpload(ctx, api.UploadRequest{
		PublicID:      enc.PublicID,
		MetadataToken: enc.MetadataToken,
		BlobToken:     blobKeys.Encoded().BlobToken,
		DeletionToken: enc.DeletionToken,
		EncryptedMeta: encryptedMeta,
		Expiration:    expiration,
		BurnAfterRead: !p.Reusable,
		BlobSize:      plan.TotalSize,
	})
	if err != nil {
		return nil, describeUploadError(err)
	}

	expiresAt, err := upload(ctx, c, session, plan, sources, blobKeys, p.Progress)
	if err != nil {
		// Whatever went wrong, the partial upload is useless: release it.
		abortCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = c.AbortUpload(abortCtx, session.ID, session.Token)
		return nil, err
	}

	return &Result{
		Link:              Link{Origin: c.BaseURL, Secret: enc.ShareSecret, DeletionToken: enc.DeletionToken},
		ExpiresAt:         expiresAt,
		Kind:              kind,
		Size:              total,
		Names:             names,
		Reusable:          p.Reusable,
		PasswordProtected: p.Password != "",
	}, nil
}

// upload encrypts the plan's records in order, groups them into parts of
// the server's size, and sends the parts a few at a time. Only a handful of
// parts are ever in memory.
func upload(ctx context.Context, c *api.Client, session *api.UploadSession, plan *bundle.Plan, sources []bundle.Source, blobKeys *keys.KeySet, progress func(uploaded, total int64)) (time.Time, error) {
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(UploadConcurrency)
	var (
		mu       sync.Mutex
		uploaded int64
	)
	report := func(n int64) {
		if progress == nil {
			return
		}
		mu.Lock()
		uploaded += n
		done := uploaded
		mu.Unlock()
		progress(done, plan.TotalSize)
	}
	report(0)

	partNumber := 1
	var partOffset int64
	part := make([]byte, 0, min(session.PartSize, plan.TotalSize)+bundle.RecordOverhead)
	flush := func() {
		data := part
		number, offset := partNumber, partOffset
		sum := bundle.SHA256Hex(data)
		g.Go(func() error {
			if err := c.UploadPart(gctx, session.ID, session.Token, number, offset, data, sum); err != nil {
				return describeUploadError(err)
			}
			report(int64(len(data)))
			return nil
		})
		partNumber++
		partOffset = offset + int64(len(data))
		part = make([]byte, 0, min(session.PartSize, plan.TotalSize-partOffset)+bundle.RecordOverhead)
	}

	plaintext := make([]byte, bundle.ChunkSize)
	for _, record := range plan.Records {
		if err := gctx.Err(); err != nil {
			// A part has failed for good, or the caller gave up: stop
			// encrypting and surface that failure.
			break
		}
		if len(part) > 0 && int64(len(part))+record.Length > session.PartSize && int64(len(part)) >= MinPartBytes {
			flush()
		}
		buf := plaintext[:record.PlaintextSize]
		if _, err := sources[record.FileIndex].Reader.ReadAt(buf, record.Start); err != nil && (!errors.Is(err, io.EOF) || int64(len(buf)) != record.PlaintextSize) {
			return time.Time{}, fmt.Errorf("read %s: %w", sources[record.FileIndex].Name, err)
		}
		encrypted, err := blobKeys.EncryptRecord(buf, bundle.ChunkAAD(record.FileIndex, record.ChunkIndex, record.PlaintextSize))
		if err != nil {
			return time.Time{}, err
		}
		if int64(len(encrypted)) != record.Length {
			return time.Time{}, errors.New("bundle record size mismatch")
		}
		part = append(part, encrypted...)
	}
	if err := gctx.Err(); err != nil {
		if werr := g.Wait(); werr != nil {
			return time.Time{}, werr
		}
		return time.Time{}, err
	}

	encryptedManifest, err := blobKeys.EncryptRecord(plan.ManifestJSON, bundle.ManifestAAD())
	if err != nil {
		return time.Time{}, err
	}
	if int64(len(encryptedManifest)) != plan.EncryptedManifestLength {
		return time.Time{}, errors.New("bundle manifest size mismatch")
	}
	var footer bundle.Footer
	footer.ManifestLength = int64(len(encryptedManifest))
	copy(footer.ManifestSHA256[:], mustDecodeHex(bundle.SHA256Hex(encryptedManifest)))
	trailer, err := bundle.EncodeFooter(footer)
	if err != nil {
		return time.Time{}, err
	}
	part = append(part, encryptedManifest...)
	part = append(part, trailer...)
	flush()

	if err := g.Wait(); err != nil {
		return time.Time{}, err
	}
	expiresAt, err := c.CompleteUpload(ctx, session.ID, session.Token)
	if err != nil {
		return time.Time{}, describeUploadError(err)
	}
	return expiresAt, nil
}

// Info is what a link reveals without opening the secret: the metadata
// envelope decrypts with the link alone.
type Info struct {
	Kind              Kind
	BundleName        string
	PasswordProtected bool
	Reusable          bool
	EncryptedSize     int64
	ExpiresAt         time.Time
	CreatedAt         time.Time
	// OpenedAt is when a recipient first opened a reusable secret, if one has.
	OpenedAt *time.Time
}

// Inspect describes a live secret, or returns a *GoneError or *NotFoundError.
func Inspect(ctx context.Context, c *api.Client, link Link) (*Info, error) {
	base, err := keys.FromShareSecret(link.Secret, "")
	if err != nil {
		return nil, err
	}
	enc := base.Encoded()
	meta, err := c.Metadata(ctx, enc.PublicID, enc.MetadataToken)
	if err != nil {
		return nil, describeLookupError(err)
	}
	clientMeta, err := base.DecryptMeta(meta.EncryptedMeta)
	if err != nil {
		return nil, ErrLinkMismatch
	}
	kind := KindFiles
	if clientMeta.Type == "text" {
		kind = KindText
	}
	return &Info{
		Kind:              kind,
		BundleName:        clientMeta.BundleName,
		PasswordProtected: clientMeta.PasswordProtected,
		Reusable:          !meta.BurnAfterRead,
		EncryptedSize:     meta.BlobSize,
		ExpiresAt:         meta.ExpiresAt,
		CreatedAt:         meta.CreatedAt,
		OpenedAt:          meta.OpenedAt,
	}, nil
}

// Sink receives what Open decrypts.
type Sink interface {
	// Text receives a text secret.
	Text(text []byte) error
	// File is asked where a file's plaintext should go. The writer is
	// closed when the file is complete.
	File(file bundle.File) (io.WriteCloser, error)
}

// ManifestSink is a Sink that wants to see the manifest before anything is
// decrypted, to check names and sizes while nothing has been written yet.
type ManifestSink interface {
	Sink
	Manifest(manifest *bundle.Manifest) error
}

// Opened is what Open found.
type Opened struct {
	Info     Info
	Manifest *bundle.Manifest
}

// Open reads a secret, handing the content to sink. Opening a one-time
// secret is what ends it. A password is needed when Info says so; a wrong
// one is ErrWrongPassword. progress, if set, is told the plaintext bytes
// written so far.
func Open(ctx context.Context, c *api.Client, link Link, password string, sink Sink, progress func(done, total int64)) (*Opened, error) {
	info, err := Inspect(ctx, c, link)
	if err != nil {
		return nil, err
	}
	if info.PasswordProtected && password == "" {
		return nil, ErrPasswordRequired
	}
	if !info.PasswordProtected {
		password = ""
	}
	blobKeys, err := keys.FromShareSecret(link.Secret, password)
	if err != nil {
		return nil, err
	}
	enc := blobKeys.Encoded()
	session, err := c.StartRetrievalSession(ctx, enc.PublicID, enc.BlobToken, link.DeletionToken)
	if err != nil {
		switch {
		case api.IsStatus(err, http.StatusForbidden) && info.PasswordProtected:
			return nil, ErrWrongPassword
		case api.IsStatus(err, http.StatusForbidden):
			return nil, ErrLinkMismatch
		case api.IsStatus(err, http.StatusNotFound):
			// It went away between the look and the opening; the server can say why.
			if _, lookErr := Inspect(ctx, c, link); lookErr != nil {
				return nil, lookErr
			}
			return nil, &NotFoundError{}
		}
		return nil, err
	}

	fetch := func(ctx context.Context, start, end int64) ([]byte, error) {
		return c.ReadRange(ctx, enc.PublicID, session.Token, start, end)
	}
	cached, err := bundle.CachingFetcher(ctx, fetch, session.BlobSize)
	if err != nil {
		return nil, describeReadError(err)
	}
	manifest, err := bundle.ReadManifest(ctx, cached, blobKeys, session.BlobSize)
	if err != nil {
		return nil, describeReadError(err)
	}
	if ms, ok := sink.(ManifestSink); ok {
		if err := ms.Manifest(manifest); err != nil {
			return nil, err
		}
	}

	total := manifest.TotalSize()
	var done int64
	for _, file := range manifest.Files {
		fileDone := done
		track := func(written int64) {
			if progress != nil {
				progress(fileDone+written, total)
			}
		}
		if info.Kind == KindText && len(manifest.Files) == 1 {
			var buf bytes.Buffer
			if err := bundle.DecryptFile(ctx, cached, blobKeys, file, &buf, track); err != nil {
				return nil, describeReadError(err)
			}
			if err := sink.Text(buf.Bytes()); err != nil {
				return nil, err
			}
		} else {
			w, err := sink.File(file)
			if err != nil {
				return nil, err
			}
			if err := bundle.DecryptFile(ctx, cached, blobKeys, file, w, track); err != nil {
				_ = w.Close()
				return nil, describeReadError(err)
			}
			if err := w.Close(); err != nil {
				return nil, err
			}
		}
		done += file.Size
	}
	return &Opened{Info: *info, Manifest: manifest}, nil
}

// Delete removes the secret for everyone. It needs the owner link.
func Delete(ctx context.Context, c *api.Client, link Link) error {
	if !link.IsOwner() {
		return ErrNotOwner
	}
	base, err := keys.FromShareSecret(link.Secret, "")
	if err != nil {
		return err
	}
	enc := base.Encoded()
	if err := c.Delete(ctx, enc.PublicID, enc.MetadataToken, link.DeletionToken); err != nil {
		if api.IsStatus(err, http.StatusForbidden) {
			return ErrLinkMismatch
		}
		return describeLookupError(err)
	}
	return nil
}

func describeLookupError(err error) error {
	if gone, ok := api.AsGone(err); ok {
		return &GoneError{Gone: *gone}
	}
	if api.IsStatus(err, http.StatusNotFound) {
		return &NotFoundError{}
	}
	if api.IsStatus(err, http.StatusForbidden) {
		return ErrLinkMismatch
	}
	return err
}

func describeReadError(err error) error {
	if api.IsStatus(err, http.StatusForbidden) {
		return errors.New("the download window has closed; open the link again")
	}
	if errors.Is(err, keys.ErrDecrypt) {
		return errors.New("the data does not decrypt with this link: it was damaged in storage or in transit")
	}
	return err
}

func describeUploadError(err error) error {
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.Status == http.StatusBadRequest && strings.Contains(apiErr.Message, "maximum size"):
			return ErrTooLarge
		case apiErr.Status == http.StatusTooManyRequests:
			return errors.New("the server is rate limiting this address; wait a minute and try again")
		}
	}
	return err
}

func mustDecodeHex(h string) []byte {
	out := make([]byte, len(h)/2)
	for i := range out {
		var b byte
		for _, c := range h[i*2 : i*2+2] {
			b <<= 4
			switch {
			case c >= '0' && c <= '9':
				b |= byte(c - '0')
			case c >= 'a' && c <= 'f':
				b |= byte(c-'a') + 10
			}
		}
		out[i] = b
	}
	return out
}
