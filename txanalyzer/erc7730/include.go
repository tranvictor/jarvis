package erc7730

// Merge applies the `includes` semantics from the spec: the parent
// descriptor (`a`) overrides the included file (`b`) on field-level
// conflicts, but lists (in particular `display.formats[*].fields`)
// merge by path so a generic ERC-20 file's approve() formatter can be
// bound to a concrete deployment without re-listing every field.
//
// The base file is mutated in place and returned; callers should pass
// a fresh deep copy or accept that the include cache will share state
// across consumers. Both arguments must be non-nil.
func Merge(base, overlay *Descriptor) *Descriptor {
	if base == nil {
		return overlay
	}
	if overlay == nil {
		return base
	}

	// Top-level scalar overrides.
	if overlay.Schema != "" {
		base.Schema = overlay.Schema
	}

	// Context: prefer overlay when present.
	if overlay.Context.Contract != nil {
		if base.Context.Contract == nil {
			base.Context.Contract = overlay.Context.Contract
		} else {
			base.Context.Contract.Deployments = append(
				base.Context.Contract.Deployments,
				overlay.Context.Contract.Deployments...,
			)
			if overlay.Context.Contract.Factory != nil {
				base.Context.Contract.Factory = overlay.Context.Contract.Factory
			}
		}
	}
	if overlay.Context.EIP712 != nil {
		if base.Context.EIP712 == nil {
			base.Context.EIP712 = overlay.Context.EIP712
		} else {
			if overlay.Context.EIP712.Domain != nil {
				base.Context.EIP712.Domain = overlay.Context.EIP712.Domain
			}
			base.Context.EIP712.Deployments = append(
				base.Context.EIP712.Deployments,
				overlay.Context.EIP712.Deployments...,
			)
			if overlay.Context.EIP712.DomainSeparator != "" {
				base.Context.EIP712.DomainSeparator = overlay.Context.EIP712.DomainSeparator
			}
		}
	}
	if overlay.Context.ID != "" {
		base.Context.ID = overlay.Context.ID
	}

	// Metadata: shallow overrides.
	if overlay.Metadata.Owner != "" {
		base.Metadata.Owner = overlay.Metadata.Owner
	}
	if overlay.Metadata.ContractName != "" {
		base.Metadata.ContractName = overlay.Metadata.ContractName
	}
	if overlay.Metadata.Info != nil {
		base.Metadata.Info = overlay.Metadata.Info
	}
	if overlay.Metadata.Token != nil {
		base.Metadata.Token = overlay.Metadata.Token
	}
	base.Metadata.Enums = mergeMap(base.Metadata.Enums, overlay.Metadata.Enums)
	base.Metadata.Constants = mergeAnyMap(base.Metadata.Constants, overlay.Metadata.Constants)
	base.Metadata.Maps = mergeMapsMap(base.Metadata.Maps, overlay.Metadata.Maps)

	// Display: definitions merge by key, formats merge by key + path.
	base.Display.Definitions = mergeFieldMap(base.Display.Definitions, overlay.Display.Definitions)
	base.Display.Formats = mergeFormats(base.Display.Formats, overlay.Display.Formats)

	return base
}

func mergeMap(a, b map[string]Enum) map[string]Enum {
	if a == nil {
		a = map[string]Enum{}
	}
	for k, v := range b {
		a[k] = v
	}
	return a
}

func mergeAnyMap(a, b map[string]any) map[string]any {
	if a == nil {
		a = map[string]any{}
	}
	for k, v := range b {
		a[k] = v
	}
	return a
}

func mergeMapsMap(a, b map[string]Map) map[string]Map {
	if a == nil {
		a = map[string]Map{}
	}
	for k, v := range b {
		a[k] = v
	}
	return a
}

func mergeFieldMap(a, b map[string]Field) map[string]Field {
	if a == nil {
		a = map[string]Field{}
	}
	for k, v := range b {
		a[k] = v
	}
	return a
}

func mergeFormats(a, b map[string]*Format) map[string]*Format {
	if a == nil {
		a = map[string]*Format{}
	}
	for k, overlay := range b {
		base, ok := a[k]
		if !ok || base == nil {
			a[k] = overlay
			continue
		}
		// Same format key in both: override scalars and merge fields by path.
		if !overlay.Intent.IsEmpty() {
			base.Intent = overlay.Intent
		}
		if overlay.InterpolatedIntent != "" {
			base.InterpolatedIntent = overlay.InterpolatedIntent
		}
		base.Fields = mergeFields(base.Fields, overlay.Fields)
		if len(overlay.Required) > 0 {
			base.Required = overlay.Required
		}
		if len(overlay.Excluded) > 0 {
			base.Excluded = overlay.Excluded
		}
		if overlay.ID != "" {
			base.ID = overlay.ID
		}
	}
	return a
}

// mergeFields merges two field lists per the spec's "merge by path"
// rule. Entries in `b` with the same Path as one in `a` override the
// fields' format/params; entries with new paths are appended after
// the base entries to preserve display order.
func mergeFields(base, overlay []Field) []Field {
	idx := map[string]int{}
	for i, f := range base {
		if f.Path != "" {
			idx[f.Path] = i
		}
	}
	for _, of := range overlay {
		if i, ok := idx[of.Path]; ok && of.Path != "" {
			base[i] = mergeField(base[i], of)
			continue
		}
		base = append(base, of)
		if of.Path != "" {
			idx[of.Path] = len(base) - 1
		}
	}
	return base
}

func mergeField(a, b Field) Field {
	out := a
	if b.Label != "" {
		out.Label = b.Label
	}
	if b.Format != "" {
		out.Format = b.Format
	}
	if b.Ref != "" {
		out.Ref = b.Ref
	}
	if b.Visible.Kind != 0 || len(b.Visible.Values) > 0 {
		out.Visible = b.Visible
	}
	if b.Separator != "" {
		out.Separator = b.Separator
	}
	if b.Iteration != "" {
		out.Iteration = b.Iteration
	}
	if b.Encryption != nil {
		out.Encryption = b.Encryption
	}
	if len(b.Fields) > 0 {
		out.Fields = mergeFields(out.Fields, b.Fields)
	}
	if out.Params == nil {
		out.Params = map[string]any{}
	}
	for k, v := range b.Params {
		out.Params[k] = v
	}
	return out
}
