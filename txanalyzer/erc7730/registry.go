package erc7730

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// LocalRegistry is the on-disk descriptor source rooted at
// ~/.jarvis/erc7730. The registry mirror (synced from
// ethereum/clear-signing-erc7730-registry) lives under
// `registry/`; user-added local descriptors live under `local/` and
// take precedence on conflict.
//
// All `.json` files anywhere under those two subtrees are eagerly
// parsed on first use; the registry is small enough (a few hundred
// KB) that walking it once per process is fine. Subsequent lookups
// are O(1) via the in-memory indexes.
type LocalRegistry struct {
	BaseDir string

	mu              sync.RWMutex
	loaded          bool
	all             []*Descriptor
	byContract      map[string][]*Descriptor // key: "<chainID>:<lower-addr>"
	byEIP712Verify  map[string][]*Descriptor // key: "<chainID>:<lower-verifyingContract>"
	byEIP712Name    map[string][]*Descriptor // key: lower-case domain.name
	bySeparator     map[string][]*Descriptor // key: lower-case domain separator hex (no 0x)
	// negativeCache records (chainId, addr) lookups that came up
	// empty against the most recent sync. Surface so callers can
	// query "have we already checked the registry for this?".
	negative map[string]int64
}

// DefaultBaseDir is ~/.jarvis/erc7730.
func DefaultBaseDir() string {
	u, err := user.Current()
	if err != nil {
		return ".jarvis/erc7730"
	}
	return filepath.Join(u.HomeDir, ".jarvis", "erc7730")
}

// NewLocalRegistry constructs a registry rooted at baseDir. baseDir
// is created on first write if it doesn't exist; nothing is read off
// disk until the first lookup.
func NewLocalRegistry(baseDir string) *LocalRegistry {
	if baseDir == "" {
		baseDir = DefaultBaseDir()
	}
	return &LocalRegistry{
		BaseDir:        baseDir,
		byContract:     map[string][]*Descriptor{},
		byEIP712Verify: map[string][]*Descriptor{},
		byEIP712Name:   map[string][]*Descriptor{},
		bySeparator:    map[string][]*Descriptor{},
		negative:       map[string]int64{},
	}
}

// FindByContract implements DescriptorSource. The (chainID, address)
// pair is normalised to a single map key. Local descriptors come
// first because we append local-loaded files before registry-loaded
// ones during Load(); selector matching downstream picks the first
// hit.
func (r *LocalRegistry) FindByContract(chainID uint64, address string) []*Descriptor {
	r.ensureLoaded()
	key := contractKey(chainID, address)
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byContract[key]
}

// FindByEIP712 implements DescriptorSource by walking three indexes:
// matching domain separator (most precise), matching verifying
// contract, and matching domain.name (least precise). De-duplicated.
func (r *LocalRegistry) FindByEIP712(td *apitypes.TypedData) []*Descriptor {
	r.ensureLoaded()
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []*Descriptor{}
	seen := map[*Descriptor]bool{}
	push := func(ds []*Descriptor) {
		for _, d := range ds {
			if seen[d] {
				continue
			}
			seen[d] = true
			out = append(out, d)
		}
	}
	// 1) DomainSeparator-keyed (live computation done by caller, we
	//    just key by stored separator; caller compares against the
	//    actual one). We surface all descriptors that carry ANY
	//    domain separator hint plus the broader candidates so the
	//    matcher gets a chance to verify them.
	for _, ds := range r.bySeparator {
		push(ds)
	}
	// 2) Verifying contract + chain.
	verifyKey := contractKey(domainChainID(td), td.Domain.VerifyingContract)
	push(r.byEIP712Verify[verifyKey])
	// 3) Domain name.
	push(r.byEIP712Name[strings.ToLower(td.Domain.Name)])
	return out
}

// LocalDir / RegistryDir are subdirectories under BaseDir.
func (r *LocalRegistry) LocalDir() string    { return filepath.Join(r.BaseDir, "local") }
func (r *LocalRegistry) RegistryDir() string { return filepath.Join(r.BaseDir, "registry") }

// ensureLoaded walks BaseDir once and populates the in-memory
// indexes. Bad files are skipped silently (they shouldn't block
// good descriptors); the only fatal error is a missing BaseDir,
// which is normal first-run and is also non-fatal.
func (r *LocalRegistry) ensureLoaded() {
	r.mu.RLock()
	if r.loaded {
		r.mu.RUnlock()
		return
	}
	r.mu.RUnlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loaded {
		return
	}
	r.loaded = true
	r.loadTreeLocked(r.LocalDir(), "local")
	r.loadTreeLocked(r.RegistryDir(), "registry")
}

func (r *LocalRegistry) loadTreeLocked(root, source string) {
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".json") {
			return nil
		}
		d, err := loadFile(path)
		if err != nil || d == nil {
			return nil
		}
		d.Source = source
		d.CachedAtUnix = info.ModTime().Unix()
		r.indexDescriptorLocked(d)
		return nil
	})
}

func (r *LocalRegistry) indexDescriptorLocked(d *Descriptor) {
	r.all = append(r.all, d)
	if d.Context.Contract != nil {
		for _, dep := range d.Context.Contract.Deployments {
			key := contractKey(dep.ChainID, dep.Address)
			r.byContract[key] = append(r.byContract[key], d)
		}
	}
	if d.Context.EIP712 != nil {
		if d.Context.EIP712.Domain != nil && d.Context.EIP712.Domain.Name != "" {
			k := strings.ToLower(d.Context.EIP712.Domain.Name)
			r.byEIP712Name[k] = append(r.byEIP712Name[k], d)
		}
		for _, dep := range d.Context.EIP712.Deployments {
			key := contractKey(dep.ChainID, dep.Address)
			r.byEIP712Verify[key] = append(r.byEIP712Verify[key], d)
		}
		if d.Context.EIP712.DomainSeparator != "" {
			k := strings.ToLower(stripHex(d.Context.EIP712.DomainSeparator))
			r.bySeparator[k] = append(r.bySeparator[k], d)
		}
	}
}

// All returns every descriptor known to the registry, for the
// `jarvis clearsign list` command.
func (r *LocalRegistry) All() []*Descriptor {
	r.ensureLoaded()
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Descriptor, len(r.all))
	copy(out, r.all)
	return out
}

// Reload drops the in-memory cache so the next lookup re-walks the
// directory tree. Used by `jarvis clearsign update` after fetching
// the registry archive and by `jarvis clearsign add` after copying
// a user-supplied file.
func (r *LocalRegistry) Reload() {
	r.mu.Lock()
	r.loaded = false
	r.all = nil
	r.byContract = map[string][]*Descriptor{}
	r.byEIP712Verify = map[string][]*Descriptor{}
	r.byEIP712Name = map[string][]*Descriptor{}
	r.bySeparator = map[string][]*Descriptor{}
	r.mu.Unlock()
}

// AddLocalFromBytes writes raw to BaseDir/local/<derived-name>.json
// after validating it parses as a descriptor. Returns the absolute
// path it wrote to.
func (r *LocalRegistry) AddLocalFromBytes(name string, raw []byte) (string, error) {
	d, err := ParseDescriptor(raw)
	if err != nil {
		return "", err
	}
	if name == "" {
		name = derivedName(d)
	}
	if name == "" {
		return "", fmt.Errorf("erc7730: cannot derive descriptor name; pass --name")
	}
	if !strings.HasSuffix(strings.ToLower(name), ".json") {
		name += ".json"
	}
	dir := r.LocalDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, raw, 0o644); err != nil {
		return "", err
	}
	r.Reload()
	return full, nil
}

// LoadFile parses a single descriptor file. Exported for the
// `clearsign show <file>` flow and for tests.
func LoadFile(path string) (*Descriptor, error) { return loadFile(path) }

func loadFile(path string) (*Descriptor, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseDescriptor(raw)
}

// ParseDescriptor parses raw bytes into a Descriptor, validating the
// minimum invariants: at least one binding context, and at least one
// format entry. We don't perform full schema validation here — the
// upstream registry already gates that.
func ParseDescriptor(raw []byte) (*Descriptor, error) {
	d := &Descriptor{}
	if err := json.Unmarshal(raw, d); err != nil {
		return nil, fmt.Errorf("erc7730: bad descriptor JSON: %w", err)
	}
	if d.Context.Contract == nil && d.Context.EIP712 == nil {
		return nil, fmt.Errorf("erc7730: descriptor missing both context.contract and context.eip712")
	}
	if len(d.Display.Formats) == 0 {
		return nil, fmt.Errorf("erc7730: descriptor has no display.formats entries")
	}
	return d, nil
}

// derivedName builds a filename from a descriptor's metadata + first
// deployment. Used by AddLocalFromBytes when the user doesn't supply
// a name.
func derivedName(d *Descriptor) string {
	name := d.Metadata.ContractName
	if name == "" {
		name = d.Metadata.Owner
	}
	if name == "" {
		name = d.Context.ID
	}
	suffix := ""
	if d.Context.Contract != nil && len(d.Context.Contract.Deployments) > 0 {
		dep := d.Context.Contract.Deployments[0]
		suffix = fmt.Sprintf("-%d-%s", dep.ChainID, strings.TrimPrefix(strings.ToLower(dep.Address), "0x"))
	} else if d.Context.EIP712 != nil && len(d.Context.EIP712.Deployments) > 0 {
		dep := d.Context.EIP712.Deployments[0]
		suffix = fmt.Sprintf("-eip712-%d-%s", dep.ChainID, strings.TrimPrefix(strings.ToLower(dep.Address), "0x"))
	} else if d.Context.EIP712 != nil && d.Context.EIP712.Domain != nil {
		suffix = "-eip712-" + slug(d.Context.EIP712.Domain.Name)
	}
	if name == "" {
		return strings.TrimPrefix(suffix, "-")
	}
	return slug(name) + suffix
}

func slug(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			out = append(out, c)
		case c >= 'A' && c <= 'Z':
			out = append(out, c+('a'-'A'))
		case c == ' ' || c == '-' || c == '_':
			out = append(out, '-')
		}
	}
	return string(out)
}

func contractKey(chainID uint64, addr string) string {
	return fmt.Sprintf("%d:%s", chainID, strings.ToLower(addr))
}
