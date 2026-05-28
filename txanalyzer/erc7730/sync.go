package erc7730

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RegistryArchiveURL points at the public mirror of the ERC-7730
// registry. We pull the tarball of the default branch to avoid
// per-file API rate limits — a fresh sync is ~one HTTP request
// regardless of how many descriptors are in the registry.
const RegistryArchiveURL = "https://github.com/ethereum/clear-signing-erc7730-registry/archive/refs/heads/master.tar.gz"

// SyncOptions controls SyncRegistry. The zero value is a sensible
// default (default URL, no progress callback).
type SyncOptions struct {
	URL      string
	Timeout  time.Duration
	OnProgress func(stage string, n int) // optional, called as files are extracted
}

// SyncRegistry downloads the registry archive and replaces
// r.RegistryDir() with its contents on success. The previous state
// is left untouched on any error (we write to a tempdir first and
// rename in one step). Returns the number of descriptors extracted.
//
// Network access is required. Callers (the `jarvis clearsign update`
// command and the lazy-on-miss path) handle offline failures
// gracefully — the local cache simply isn't refreshed.
func (r *LocalRegistry) SyncRegistry(ctx context.Context, opts SyncOptions) (int, error) {
	url := opts.URL
	if url == "" {
		url = RegistryArchiveURL
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("erc7730: registry fetch %s: %s", url, resp.Status)
	}

	tmp, err := os.MkdirTemp(r.BaseDir, ".sync-*")
	if err != nil {
		// First-run: BaseDir may not exist yet.
		if err := os.MkdirAll(r.BaseDir, 0o755); err != nil {
			return 0, err
		}
		tmp, err = os.MkdirTemp(r.BaseDir, ".sync-*")
		if err != nil {
			return 0, err
		}
	}
	defer os.RemoveAll(tmp)

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return 0, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	count := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := hdr.Name
		// The archive structure is
		//   clear-signing-erc7730-registry-main/registry/<owner>/<file>.json
		// — we keep only files under `registry/` and strip the
		// archive root so we land at registry/<owner>/<file>.json
		// inside our destination.
		idx := strings.Index(name, "/registry/")
		if idx < 0 {
			continue
		}
		rel := name[idx+1:] // "registry/<owner>/<file>.json"
		base := filepath.Base(rel)
		if !strings.HasSuffix(strings.ToLower(base), ".json") ||
			strings.HasSuffix(strings.ToLower(base), ".tests.json") {
			continue
		}
		dst := filepath.Join(tmp, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return 0, err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return 0, err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return 0, err
		}
		out.Close()
		count++
		if opts.OnProgress != nil && count%25 == 0 {
			opts.OnProgress("extract", count)
		}
	}

	// Atomic swap: rename old registry to a sibling tombstone, then
	// rename the new one into place. If we crash mid-swap, jarvis
	// finds either the previous OR the new registry, never a
	// half-empty one.
	dst := r.RegistryDir()
	tombstone := dst + ".old"
	_ = os.RemoveAll(tombstone)
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, tombstone); err != nil {
			return 0, err
		}
	}
	srcRegistry := filepath.Join(tmp, "registry")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		_ = os.Rename(tombstone, dst)
		return 0, err
	}
	if err := os.Rename(srcRegistry, dst); err != nil {
		_ = os.Rename(tombstone, dst)
		return 0, err
	}
	_ = os.RemoveAll(tombstone)
	r.Reload()
	return count, nil
}

// LastSyncFile returns the path of the small marker file we touch
// every time the registry is synced. Used by callers to decide
// whether a "stale on miss" lazy refresh is appropriate.
func (r *LocalRegistry) LastSyncFile() string {
	return filepath.Join(r.BaseDir, ".last-sync")
}

// TouchLastSync writes the current time to LastSyncFile. Called after
// a successful SyncRegistry. We use the file's mtime, not its body,
// so there's no parsing required to query the freshness.
func (r *LocalRegistry) TouchLastSync() {
	_ = os.MkdirAll(r.BaseDir, 0o755)
	now := time.Now()
	f, err := os.Create(r.LastSyncFile())
	if err != nil {
		return
	}
	f.Close()
	_ = os.Chtimes(r.LastSyncFile(), now, now)
}

// LastSyncAge returns how long ago the registry was last synced, or
// a very large duration on first run / read errors so callers will
// trigger a fresh sync.
func (r *LocalRegistry) LastSyncAge() time.Duration {
	info, err := os.Stat(r.LastSyncFile())
	if err != nil {
		return 365 * 24 * time.Hour
	}
	return time.Since(info.ModTime())
}
