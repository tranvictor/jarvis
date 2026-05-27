package erc7730

import "strings"

// interpolate replaces every "{path}" placeholder in template with the
// formatted value of the corresponding field. The spec guarantees
// each placeholder refers to a path that also has a `fields` entry
// (the wallet MUST format placeholders identically to standalone
// fields), so we look the path up by ranging over fields rather than
// re-parsing the path each time.
//
// Curly braces are escaped by doubling per the spec.
func interpolate(template string, fields []Field, f *Formatter) string {
	if template == "" {
		return ""
	}
	var out strings.Builder
	out.Grow(len(template))
	i := 0
	for i < len(template) {
		c := template[i]
		// Escaped "{{" / "}}" → literal "{" / "}".
		if i+1 < len(template) && (c == '{' && template[i+1] == '{' || c == '}' && template[i+1] == '}') {
			out.WriteByte(c)
			i += 2
			continue
		}
		if c == '{' {
			end := strings.IndexByte(template[i+1:], '}')
			if end < 0 {
				out.WriteByte(c)
				i++
				continue
			}
			path := template[i+1 : i+1+end]
			out.WriteString(resolvePlaceholder(path, fields, f))
			i = i + 1 + end + 1
			continue
		}
		out.WriteByte(c)
		i++
	}
	return out.String()
}

func resolvePlaceholder(path string, fields []Field, f *Formatter) string {
	// Find the field spec whose path matches (with relaxed
	// rooted/relative comparison).
	want := normalizePathForCompare(path)
	for _, fld := range fields {
		if normalizePathForCompare(fld.Path) != want {
			continue
		}
		rows, err := f.FormatField(fld)
		if err != nil || len(rows) == 0 {
			return "{" + path + "}"
		}
		return rows[0].Value
	}
	// No field formatter → fall back to a raw resolve so common
	// placeholders like {to} still print something.
	if parsed, err := ParsePath(path); err == nil {
		if val, err := f.Resolver.Resolve(parsed); err == nil {
			return f.formatRaw(val)
		}
	}
	return "{" + path + "}"
}

// normalizePathForCompare ignores the leading root marker so a
// descriptor that writes "to" matches a placeholder written "#.to"
// (the spec allows both).
func normalizePathForCompare(s string) string {
	if len(s) == 0 {
		return s
	}
	if s[0] == '#' || s[0] == '$' || s[0] == '@' {
		s = s[1:]
		s = strings.TrimPrefix(s, ".")
	}
	return s
}
