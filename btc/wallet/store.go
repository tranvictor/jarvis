package wallet

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var ErrNotFound = errors.New("no such bitcoin wallet")

// Dir is ~/.jarvis/btc. $HOME is honoured so tests and containers can
// redirect it.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".jarvis", "btc"), nil
}

func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func Save(rec Record, overwrite bool) error {
	if rec.Name == "" || Slug(rec.Name) != rec.Name {
		return fmt.Errorf("invalid wallet name %q", rec.Name)
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, rec.Name+".json")
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("a bitcoin wallet named %q already exists", rec.Name)
		}
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func List() ([]Record, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Record
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var rec Record
		if err := json.Unmarshal(data, &rec); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Resolve finds a wallet by exact name or description first, then by a
// unique prefix. Ambiguity is an error: signing from the wrong wallet is
// worse than asking again.
func Resolve(keyword string) (Record, error) {
	recs, err := List()
	if err != nil {
		return Record{}, err
	}
	kw := strings.ToLower(strings.TrimSpace(keyword))
	slug := Slug(keyword)
	for _, r := range recs {
		if r.Name == slug || strings.EqualFold(r.Desc, kw) || strings.EqualFold(r.Address, kw) {
			return r, nil
		}
	}
	var hits []Record
	for _, r := range recs {
		if slug != "" && strings.HasPrefix(r.Name, slug) {
			hits = append(hits, r)
		}
	}
	switch len(hits) {
	case 0:
		return Record{}, fmt.Errorf("%w matching %q (see: jarvis btc wallet list)", ErrNotFound, keyword)
	case 1:
		return hits[0], nil
	}
	names := make([]string, len(hits))
	for i, h := range hits {
		names[i] = h.Name
	}
	return Record{}, fmt.Errorf("%q matches several bitcoin wallets (%s); be more specific", keyword, strings.Join(names, ", "))
}
