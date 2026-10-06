package bundle

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"

	"github.com/pscheid92/secretli/internal/share/keys"
)

// RangeFetcher reads the bundle bytes from start to end, both inclusive.
type RangeFetcher func(ctx context.Context, start, end int64) ([]byte, error)

// CachingFetcher fetches a small bundle once and serves every later range
// from memory; a large bundle is passed through untouched.
func CachingFetcher(ctx context.Context, fetch RangeFetcher, bundleSize int64) (RangeFetcher, error) {
	if bundleSize > SmallBundleBytes {
		return fetch, nil
	}
	whole, err := fetch(ctx, 0, bundleSize-1)
	if err != nil {
		return nil, err
	}
	if int64(len(whole)) != bundleSize {
		return nil, ErrRangeMismatch
	}
	return func(_ context.Context, start, end int64) ([]byte, error) {
		if start < 0 || end >= bundleSize || end < start {
			return nil, ErrRangeMismatch
		}
		return whole[start : end+1], nil
	}, nil
}

// ReadManifest fetches the footer and the manifest and checks both.
func ReadManifest(ctx context.Context, fetch RangeFetcher, ks *keys.KeySet, bundleSize int64) (*Manifest, error) {
	if bundleSize < FooterLength {
		return nil, ErrInvalidFooter
	}
	trailer, err := fetch(ctx, bundleSize-FooterLength, bundleSize-1)
	if err != nil {
		return nil, err
	}
	footer, err := ParseFooter(trailer)
	if err != nil {
		return nil, err
	}
	if footer.ManifestLength > MaxManifestBytes+RecordOverhead {
		return nil, ErrManifestTooLarge
	}
	manifestOffset := bundleSize - FooterLength - footer.ManifestLength
	if manifestOffset < 0 {
		return nil, ErrInvalidFooter
	}
	encrypted, err := fetch(ctx, manifestOffset, manifestOffset+footer.ManifestLength-1)
	if err != nil {
		return nil, err
	}
	if int64(len(encrypted)) != footer.ManifestLength {
		return nil, ErrRangeMismatch
	}
	if sha256.Sum256(encrypted) != footer.ManifestSHA256 {
		return nil, fmt.Errorf("%w: hash mismatch", ErrInvalidManifest)
	}
	plaintext, err := ks.DecryptRecord(encrypted, ManifestAAD())
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(plaintext, &manifest); err != nil {
		return nil, ErrInvalidManifest
	}
	if err := manifest.validate(manifestOffset, bundleSize); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// DecryptFile streams one file's plaintext to w, fetching neighbouring
// records together up to CoalesceBytes of plaintext per request. progress,
// if set, is told the plaintext bytes written so far.
func DecryptFile(ctx context.Context, fetch RangeFetcher, ks *keys.KeySet, file File, w io.Writer, progress func(written int64)) error {
	var written int64
	for start := 0; start < len(file.Chunks); {
		// Records of one file are laid out back to back, so a run of them is
		// one contiguous range.
		end := start
		groupPlaintext := file.Chunks[start].PlaintextSize
		for end+1 < len(file.Chunks) && groupPlaintext+file.Chunks[end+1].PlaintextSize <= CoalesceBytes {
			end++
			groupPlaintext += file.Chunks[end].PlaintextSize
		}
		first, last := file.Chunks[start], file.Chunks[end]
		groupLength := last.Offset + last.Length - first.Offset
		encrypted, err := fetch(ctx, first.Offset, first.Offset+groupLength-1)
		if err != nil {
			return err
		}
		if int64(len(encrypted)) != groupLength {
			return ErrRangeMismatch
		}
		var cursor int64
		for _, chunk := range file.Chunks[start : end+1] {
			record := encrypted[cursor : cursor+chunk.Length]
			// No separate checksum: every record carries a Poly1305 tag bound
			// to its position, so any modified byte fails here.
			plaintext, err := ks.DecryptRecord(record, ChunkAAD(file.Index, chunk.Index, chunk.PlaintextSize))
			if err != nil {
				return err
			}
			if int64(len(plaintext)) != chunk.PlaintextSize {
				return fmt.Errorf("%w: chunk size", ErrInvalidManifest)
			}
			if _, err := w.Write(plaintext); err != nil {
				return err
			}
			written += int64(len(plaintext))
			if progress != nil {
				progress(written)
			}
			cursor += chunk.Length
		}
		start = end + 1
	}
	return nil
}
