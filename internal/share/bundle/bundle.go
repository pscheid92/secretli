// Package bundle is the encrypted blob format: files cut into 4 MiB records,
// each sealed and bound to its position, followed by an encrypted manifest
// and a fixed footer that says where the manifest is. It mirrors
// web/frontend/src/lib/bundle.ts; the vectors test checks the two against
// each other.
package bundle

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/pscheid92/secretli/internal/share/keys"
)

const (
	// FooterLength is the fixed size of the trailer at the end of a bundle.
	FooterLength = 64
	// ChunkSize is the plaintext size of every record but a file's last.
	ChunkSize = 4 * 1024 * 1024
	// MaxManifestBytes caps the manifest; very many files overflow it.
	MaxManifestBytes = 256 * 1024
	// RecordOverhead is what encryption adds to each record.
	RecordOverhead = keys.RecordOverhead
	// CoalesceBytes bounds how much plaintext one range request fetches when
	// reading: neighbouring records are read together up to this much.
	CoalesceBytes = 16 * 1024 * 1024
	// SmallBundleBytes: bundles up to this size are fetched whole, in one
	// request, instead of footer, manifest and records separately.
	SmallBundleBytes = 1024 * 1024

	version = 2
)

var magic = []byte{0x53, 0x4c, 0x42, 0x4e, 0x44, 0x4c, 0x32, 0x00}

var (
	// ErrInvalidFooter is a trailer that is not a bundle footer.
	ErrInvalidFooter = errors.New("invalid bundle footer")
	// ErrInvalidManifest is a manifest that does not describe this bundle.
	ErrInvalidManifest = errors.New("invalid bundle manifest")
	// ErrManifestTooLarge is a bundle whose manifest would exceed the cap.
	ErrManifestTooLarge = errors.New("bundle manifest is too large")
	// ErrEmpty is a bundle without files.
	ErrEmpty = errors.New("bundle must contain at least one file")
	// ErrRangeMismatch is a range read that returned the wrong number of bytes.
	ErrRangeMismatch = errors.New("bundle range size mismatch")
)

// Chunk is one record of a file inside the bundle.
type Chunk struct {
	Index         int   `json:"index"`
	Offset        int64 `json:"offset"`
	Length        int64 `json:"length"`
	PlaintextSize int64 `json:"plaintextSize"`
}

// File is one file of the bundle as the manifest describes it.
type File struct {
	Index  int     `json:"index"`
	Path   string  `json:"path"`
	Name   string  `json:"name"`
	Type   string  `json:"type"`
	Size   int64   `json:"size"`
	Chunks []Chunk `json:"chunks"`
}

// Manifest lists the files and where their records are.
type Manifest struct {
	Version    int    `json:"version"`
	BundleName string `json:"bundleName"`
	ChunkSize  int64  `json:"chunkSize"`
	Files      []File `json:"files"`
}

// TotalSize is the plaintext size of every file together.
func (m *Manifest) TotalSize() int64 {
	var total int64
	for _, f := range m.Files {
		total += f.Size
	}
	return total
}

// Footer is the fixed trailer: where the encrypted manifest is and its hash.
type Footer struct {
	ManifestLength int64
	ManifestSHA256 [sha256.Size]byte
}

// Source is a file to bundle. Bytes are read by position, so a file can be
// encrypted record by record without being held in memory.
type Source struct {
	Name string
	// Path is what the manifest records; it defaults to Name.
	Path string
	// Type is the MIME type; it defaults to application/octet-stream.
	Type   string
	Size   int64
	Reader io.ReaderAt
}

// Record is one encryption unit of the plan: a slice of a file and where its
// ciphertext goes in the bundle.
type Record struct {
	FileIndex     int
	ChunkIndex    int
	Start         int64
	End           int64
	Offset        int64
	Length        int64
	PlaintextSize int64
}

// Plan is the layout of a bundle before it is encrypted.
type Plan struct {
	Manifest Manifest
	// ManifestJSON is the manifest exactly as it will be encrypted.
	ManifestJSON []byte
	Records      []Record
	// DataSize is the size of all records; the manifest and footer follow.
	DataSize                int64
	EncryptedManifestLength int64
	// TotalSize is the size of the finished bundle, declared to the server.
	TotalSize int64
}

// NewPlan lays out the bundle for these sources.
func NewPlan(sources []Source, bundleName string) (*Plan, error) {
	if len(sources) == 0 {
		return nil, ErrEmpty
	}
	var offset int64
	records := []Record{}
	files := make([]File, 0, len(sources))
	for fileIndex, src := range sources {
		chunks := make([]Chunk, 0)
		for start, chunkIndex := int64(0), 0; start < src.Size; start, chunkIndex = start+ChunkSize, chunkIndex+1 {
			end := min(start+ChunkSize, src.Size)
			plaintextSize := end - start
			length := plaintextSize + RecordOverhead
			records = append(records, Record{
				FileIndex: fileIndex, ChunkIndex: chunkIndex,
				Start: start, End: end, Offset: offset, Length: length, PlaintextSize: plaintextSize,
			})
			chunks = append(chunks, Chunk{Index: chunkIndex, Offset: offset, Length: length, PlaintextSize: plaintextSize})
			offset += length
		}
		path := src.Path
		if path == "" {
			path = src.Name
		}
		typ := src.Type
		if typ == "" {
			typ = "application/octet-stream"
		}
		files = append(files, File{Index: fileIndex, Path: path, Name: src.Name, Type: typ, Size: src.Size, Chunks: chunks})
	}

	manifest := Manifest{Version: version, BundleName: bundleName, ChunkSize: ChunkSize, Files: files}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	if len(manifestJSON) > MaxManifestBytes {
		return nil, ErrManifestTooLarge
	}
	encryptedManifestLength := int64(len(manifestJSON)) + RecordOverhead
	return &Plan{
		Manifest:                manifest,
		ManifestJSON:            manifestJSON,
		Records:                 records,
		DataSize:                offset,
		EncryptedManifestLength: encryptedManifestLength,
		TotalSize:               offset + encryptedManifestLength + FooterLength,
	}, nil
}

// DefaultBundleName is what the web app calls a bundle of these files.
func DefaultBundleName(names []string) string {
	if len(names) == 1 {
		if names[0] != "" {
			return names[0]
		}
		return "Secretli file"
	}
	return fmt.Sprintf("Secretli bundle (%d files)", len(names))
}

// EstimateEncryptedSize is an upper bound on the bundle size for files of
// these sizes, used to check the upload limit before planning.
func EstimateEncryptedSize(sizes []int64) int64 {
	var chunks, plaintext int64
	for _, size := range sizes {
		chunks += (size + ChunkSize - 1) / ChunkSize
		plaintext += size
	}
	return plaintext + chunks*RecordOverhead + MaxManifestBytes + RecordOverhead + FooterLength
}

// ManifestAAD binds the manifest record to its role.
func ManifestAAD() []byte {
	return []byte("manifest:v2")
}

// ChunkAAD binds a record to its file, position and size.
func ChunkAAD(fileIndex, chunkIndex int, plaintextSize int64) []byte {
	return fmt.Appendf(nil, "chunk:%d:%d:%d", fileIndex, chunkIndex, plaintextSize)
}

// EncodeFooter writes the 64-byte trailer: magic, version, footer length,
// manifest length, the manifest's SHA-256, and zero padding.
func EncodeFooter(f Footer) ([]byte, error) {
	if f.ManifestLength <= RecordOverhead {
		return nil, ErrInvalidFooter
	}
	out := make([]byte, FooterLength)
	copy(out, magic)
	binary.BigEndian.PutUint32(out[8:], version)
	binary.BigEndian.PutUint32(out[12:], FooterLength)
	binary.BigEndian.PutUint64(out[16:], uint64(f.ManifestLength)) //nolint:gosec // checked positive above
	copy(out[24:], f.ManifestSHA256[:])
	return out, nil
}

// ParseFooter reads a trailer written by EncodeFooter.
func ParseFooter(b []byte) (Footer, error) {
	if len(b) != FooterLength || string(b[:len(magic)]) != string(magic) {
		return Footer{}, ErrInvalidFooter
	}
	if binary.BigEndian.Uint32(b[8:]) != version || binary.BigEndian.Uint32(b[12:]) != FooterLength {
		return Footer{}, ErrInvalidFooter
	}
	length := binary.BigEndian.Uint64(b[16:])
	if length > 1<<53 || int64(length) <= RecordOverhead { //nolint:gosec // bounded just before
		return Footer{}, ErrInvalidFooter
	}
	var f Footer
	f.ManifestLength = int64(length) //nolint:gosec // bounded above
	copy(f.ManifestSHA256[:], b[24:56])
	return f, nil
}

// SHA256Hex is the lower-case hex digest the format and the API use.
func SHA256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// validate checks a manifest against the bundle it came from: the files'
// records must tile the bytes before the manifest exactly.
func (m *Manifest) validate(manifestOffset, bundleSize int64) error {
	if m.Version != version || m.ChunkSize != ChunkSize || m.BundleName == "" || len(m.Files) == 0 ||
		manifestOffset < 0 || manifestOffset+FooterLength > bundleSize {
		return ErrInvalidManifest
	}
	type span struct{ start, end int64 }
	var spans []span
	for fileIndex, f := range m.Files {
		if f.Index != fileIndex || f.Path == "" || f.Name == "" || f.Size < 0 {
			return ErrInvalidManifest
		}
		var fileSize int64
		for chunkIndex, c := range f.Chunks {
			if c.Index != chunkIndex || c.Offset < 0 || c.PlaintextSize <= 0 || c.PlaintextSize > m.ChunkSize ||
				c.Length != c.PlaintextSize+RecordOverhead {
				return ErrInvalidManifest
			}
			end := c.Offset + c.Length - 1
			if end >= manifestOffset {
				return ErrInvalidManifest
			}
			spans = append(spans, span{c.Offset, end})
			fileSize += c.PlaintextSize
		}
		if fileSize != f.Size || (f.Size == 0 && len(f.Chunks) != 0) {
			return ErrInvalidManifest
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	var expected int64
	for _, s := range spans {
		if s.start != expected {
			return ErrInvalidManifest
		}
		expected = s.end + 1
	}
	if expected != manifestOffset {
		return ErrInvalidManifest
	}
	return nil
}
