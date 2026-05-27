package erc7730

import (
	"fmt"
	"strconv"
	"strings"
)

// Path is a parsed ERC-7730 reference path (the spec calls these "path
// references"). It uses a deliberately limited subset of JSON-path:
//
//   - Roots: # (structured data), $ (descriptor file), @ (container)
//   - Dot notation only: `params.amountIn`
//   - Array index: `recipients.[0]` (or bare `[0]` after a dot)
//   - Negative indices: `[-1]` is the last element
//   - Array slices: `[start:end]`, `[start:]`, `[:end]`, `[:]`, `[-N:]`
//   - Trailing slice on a primitive (`bytes`, `string`) selects bytes
//
// Anything else (filters, wildcards, scripting) is out of scope. The
// parser stays strict and returns an error rather than guessing.
type Path struct {
	Root     PathRoot
	Segments []PathSeg
	Original string
}

// PathRoot enumerates the four possible roots.
type PathRoot int

const (
	RootStruct    PathRoot = iota // # — decoded structured data (default for unprefixed paths)
	RootDesc                      // $ — the ERC-7730 descriptor file itself
	RootContainer                 // @ — container values (tx / message metadata)
)

// PathSeg is one segment of a path.
type PathSeg struct {
	Kind PathSegKind
	// Name is set for Field segments.
	Name string
	// Index is set for IndexAt segments.
	Index int
	// Start/End/HasStart/HasEnd are set for Slice segments. End is exclusive.
	Start    int
	End      int
	HasStart bool
	HasEnd   bool
}

type PathSegKind int

const (
	SegField   PathSegKind = iota // .name
	SegIndexAt                    // .[N]
	SegSlice                      // .[a:b]
)

// ParsePath parses a path reference per the ERC-7730 path subset.
// Empty input yields an error.
func ParsePath(s string) (Path, error) {
	if s == "" {
		return Path{}, fmt.Errorf("erc7730: empty path")
	}
	p := Path{Original: s}
	rest := s
	switch rest[0] {
	case '#':
		p.Root = RootStruct
		rest = rest[1:]
	case '$':
		p.Root = RootDesc
		rest = rest[1:]
	case '@':
		p.Root = RootContainer
		rest = rest[1:]
	default:
		p.Root = RootStruct
	}
	rest = strings.TrimPrefix(rest, ".")

	if rest == "" {
		return p, nil
	}

	for _, raw := range splitPathSegments(rest) {
		seg, err := parseSeg(raw)
		if err != nil {
			return Path{}, fmt.Errorf("erc7730: bad path %q: %w", s, err)
		}
		p.Segments = append(p.Segments, seg)
	}
	return p, nil
}

// splitPathSegments splits "params.path.[0:20]" into
// ["params", "path", "[0:20]"]. Bracketed segments stay glued to the
// surrounding name when the spec embeds them inline (e.g. "recipients[0]"
// vs "recipients.[0]") — we normalise both forms by splitting on dots
// and recognising the leading "[" prefix.
func splitPathSegments(s string) []string {
	var out []string
	for _, raw := range strings.Split(s, ".") {
		if raw == "" {
			continue
		}
		// Handle "recipients[0]" → "recipients", "[0]"
		if i := strings.IndexByte(raw, '['); i > 0 {
			out = append(out, raw[:i])
			raw = raw[i:]
		}
		out = append(out, raw)
	}
	return out
}

func parseSeg(s string) (PathSeg, error) {
	if strings.HasPrefix(s, "[") {
		if !strings.HasSuffix(s, "]") {
			return PathSeg{}, fmt.Errorf("unterminated bracket: %q", s)
		}
		inside := s[1 : len(s)-1]
		if strings.Contains(inside, ":") {
			parts := strings.SplitN(inside, ":", 2)
			seg := PathSeg{Kind: SegSlice}
			if parts[0] != "" {
				n, err := strconv.Atoi(parts[0])
				if err != nil {
					return PathSeg{}, fmt.Errorf("bad slice start %q", parts[0])
				}
				seg.Start = n
				seg.HasStart = true
			}
			if parts[1] != "" {
				n, err := strconv.Atoi(parts[1])
				if err != nil {
					return PathSeg{}, fmt.Errorf("bad slice end %q", parts[1])
				}
				seg.End = n
				seg.HasEnd = true
			}
			return seg, nil
		}
		n, err := strconv.Atoi(inside)
		if err != nil {
			return PathSeg{}, fmt.Errorf("bad index %q", inside)
		}
		return PathSeg{Kind: SegIndexAt, Index: n}, nil
	}
	return PathSeg{Kind: SegField, Name: s}, nil
}

// IsLeafSlice reports whether the final segment is a slice. Used by
// the formatter layer to decide if a [0:20] suffix on a bytes value
// should narrow the displayed bytes.
func (p Path) IsLeafSlice() bool {
	if len(p.Segments) == 0 {
		return false
	}
	return p.Segments[len(p.Segments)-1].Kind == SegSlice
}

// Append returns a new path with seg appended; the receiver is not
// mutated. Used for resolving relative paths inside groups.
func (p Path) Append(seg PathSeg) Path {
	np := p
	np.Segments = append([]PathSeg{}, p.Segments...)
	np.Segments = append(np.Segments, seg)
	return np
}

// Join returns p with other's segments appended. The root of `other`
// is ignored — Join is used to make a child group's relative path
// absolute under its parent.
func (p Path) Join(other Path) Path {
	np := p
	np.Segments = append([]PathSeg{}, p.Segments...)
	np.Segments = append(np.Segments, other.Segments...)
	return np
}

// String reconstructs the canonical form of the path.
func (p Path) String() string {
	var b strings.Builder
	switch p.Root {
	case RootStruct:
		b.WriteByte('#')
	case RootDesc:
		b.WriteByte('$')
	case RootContainer:
		b.WriteByte('@')
	}
	for _, seg := range p.Segments {
		switch seg.Kind {
		case SegField:
			b.WriteByte('.')
			b.WriteString(seg.Name)
		case SegIndexAt:
			b.WriteString(".[")
			b.WriteString(strconv.Itoa(seg.Index))
			b.WriteByte(']')
		case SegSlice:
			b.WriteString(".[")
			if seg.HasStart {
				b.WriteString(strconv.Itoa(seg.Start))
			}
			b.WriteByte(':')
			if seg.HasEnd {
				b.WriteString(strconv.Itoa(seg.End))
			}
			b.WriteByte(']')
		}
	}
	return b.String()
}
