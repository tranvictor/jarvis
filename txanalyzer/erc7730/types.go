package erc7730

import (
	"encoding/json"
	"fmt"
)

// Descriptor is the top-level structure of an ERC-7730 JSON file
// (https://eips.ethereum.org/EIPS/eip-7730). It is parsed from the
// JSON shape described in the spec; field names below mirror the
// spec's JSON keys, just promoted to Go exports.
type Descriptor struct {
	Schema   string   `json:"$schema,omitempty"`
	Includes string   `json:"includes,omitempty"`
	Context  Context  `json:"context"`
	Metadata Metadata `json:"metadata"`
	Display  Display  `json:"display"`

	// Source records where this descriptor came from for provenance
	// reporting in the rendered view ("registry", "local", "user").
	Source string `json:"-"`
	// CachedAt is the on-disk mtime of the file the descriptor was
	// loaded from. Surfaced to the user under the intent line.
	CachedAtUnix int64 `json:"-"`
}

// Context is the binding context — describes which structured data
// this descriptor applies to. Only one of Contract / EIP712 is set.
type Context struct {
	ID       string          `json:"$id,omitempty"`
	Contract *ContractCtx    `json:"contract,omitempty"`
	EIP712   *EIP712Ctx      `json:"eip712,omitempty"`
}

// ContractCtx binds a descriptor to EVM smart contract calldata.
type ContractCtx struct {
	Deployments []Deployment `json:"deployments"`

	// Factory describes how to recognise contracts deployed by a
	// known factory — used for clear-signing pools / vaults that
	// only exist once a factory event has fired.
	Factory *Factory `json:"factory,omitempty"`

	// ABI is deprecated by the spec but still appears in legacy
	// files. We tolerate it on parse but never consult it.
	ABI any `json:"abi,omitempty"`
}

// Deployment is one (chainId, address) tuple a Contract / EIP712
// descriptor binds to. A descriptor may list many deployments.
type Deployment struct {
	ChainID uint64 `json:"chainId"`
	Address string `json:"address"`
}

// Factory binds a descriptor to all contracts deployed by a known
// factory contract.
type Factory struct {
	// DeployEvent is the Solidity-format event signature whose first
	// indexed address argument is the deployed contract address —
	// e.g. "PoolCreated(address indexed pool, ...)".
	DeployEvent string       `json:"deployEvent"`
	Deployments []Deployment `json:"deployments"`
}

// EIP712Ctx binds a descriptor to EIP-712 typed-data messages.
type EIP712Ctx struct {
	Domain          *EIP712Domain `json:"domain,omitempty"`
	Deployments     []Deployment  `json:"deployments,omitempty"`
	DomainSeparator string        `json:"domainSeparator,omitempty"`

	// Schemas is the deprecated legacy attachment; we tolerate it
	// but read schemas from display.formats keys instead.
	Schemas any `json:"schemas,omitempty"`
}

// EIP712Domain captures the subset of the message domain a descriptor
// requires to match. All fields are optional; only set ones are
// checked.
type EIP712Domain struct {
	Name              string `json:"name,omitempty"`
	Version           string `json:"version,omitempty"`
	ChainID           uint64 `json:"chainId,omitempty"`
	VerifyingContract string `json:"verifyingContract,omitempty"`
}

// Metadata carries displayable constants (owner, contractName, token,
// enums, maps, constants). All optional.
type Metadata struct {
	Owner        string           `json:"owner,omitempty"`
	ContractName string           `json:"contractName,omitempty"`
	Info         *MetadataInfo    `json:"info,omitempty"`
	Token        *TokenMetadata   `json:"token,omitempty"`
	Enums        map[string]Enum  `json:"enums,omitempty"`
	Constants    map[string]any   `json:"constants,omitempty"`
	Maps         map[string]Map   `json:"maps,omitempty"`
}

type MetadataInfo struct {
	URL            string `json:"url,omitempty"`
	DeploymentDate string `json:"deploymentDate,omitempty"`
	LegalName      string `json:"legalName,omitempty"`
	LastUpdate     string `json:"lastUpdate,omitempty"`
}

// TokenMetadata is metadata.token, used when the contract is itself
// an ERC-20 that doesn't expose name()/symbol()/decimals() on-chain.
// When it IS exposed, the spec says descriptors SHOULD omit this.
type TokenMetadata struct {
	Name     string `json:"name,omitempty"`
	Ticker   string `json:"ticker,omitempty"`
	Decimals uint64 `json:"decimals,omitempty"`
}

// Enum is a flat key→display-string mapping used by the "enum"
// format. Keys are usually small integers as strings.
type Enum map[string]string

// Map is a context-dependent constant table referenced by formatter
// params via {map: "$.metadata.maps.<name>", keyPath: "@.chainId"}.
type Map struct {
	KeyType string            `json:"$keyType,omitempty"`
	Values  map[string]string `json:"values"`
}

// Display is the formatting block.
type Display struct {
	Definitions map[string]Field   `json:"definitions,omitempty"`
	Formats     map[string]*Format `json:"formats"`
}

// Format describes how to render one function call (for contract
// descriptors) or one EIP-712 message type (for eip712 descriptors).
// The map key is the canonical signature.
type Format struct {
	ID                 string  `json:"$id,omitempty"`
	Intent             Intent  `json:"intent,omitempty"`
	InterpolatedIntent string  `json:"interpolatedIntent,omitempty"`
	Fields             []Field `json:"fields,omitempty"`

	// Required / Excluded are advisory: the spec uses them to mark
	// fields that MUST be present in the interpolated intent, and
	// fields the renderer SHOULD skip even if they have a value.
	Required []string `json:"required,omitempty"`
	Excluded []string `json:"excluded,omitempty"`
}

// Intent is either a plain string or a flat key/value map. We carry
// the original JSON and surface whichever shape it had through the
// accessor methods.
type Intent struct {
	Text string
	Map  map[string]string
}

// UnmarshalJSON accepts both representations the spec allows.
func (i *Intent) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		i.Text = s
		return nil
	}
	if b[0] == '{' {
		m := map[string]string{}
		if err := json.Unmarshal(b, &m); err != nil {
			return err
		}
		i.Map = m
		return nil
	}
	return fmt.Errorf("erc7730: intent must be string or object, got %s", string(b))
}

// MarshalJSON re-emits whichever shape was parsed in.
func (i Intent) MarshalJSON() ([]byte, error) {
	if i.Map != nil {
		return json.Marshal(i.Map)
	}
	return json.Marshal(i.Text)
}

// IsEmpty reports whether neither representation was provided.
func (i Intent) IsEmpty() bool { return i.Text == "" && len(i.Map) == 0 }

// Field is one entry in Format.Fields. The same struct represents
// both leaf field formatters and group formatters: a group is a
// Field with nested Fields set.
type Field struct {
	Path     string         `json:"path,omitempty"`
	Value    any            `json:"value,omitempty"`
	Ref      string         `json:"$ref,omitempty"`
	Label    string         `json:"label,omitempty"`
	Format   string         `json:"format,omitempty"`
	Params   map[string]any `json:"params,omitempty"`
	ID       string         `json:"$id,omitempty"`
	Visible  Visibility     `json:"visible,omitempty"`
	Separator string        `json:"separator,omitempty"`

	// Iteration applies only to group fields containing array paths.
	// "sequential" (default) iterates each array independently;
	// "bundled" zips them element-wise.
	Iteration string `json:"iteration,omitempty"`

	// Encryption is parsed but currently ignored — FHE / ERC-7984
	// fields fall through to the raw display.
	Encryption *EncryptionInfo `json:"encryption,omitempty"`

	// Fields is the recursive group case.
	Fields []Field `json:"fields,omitempty"`
}

// EncryptionInfo captures the optional encryption hint on a field.
type EncryptionInfo struct {
	Scheme        string `json:"scheme"`
	PlaintextType string `json:"plaintextType"`
	FallbackLabel string `json:"fallbackLabel,omitempty"`
}

// VisibilityKind enumerates the simple-form visibility states.
type VisibilityKind int

const (
	VisibilityAlways   VisibilityKind = iota // shown (the spec default)
	VisibilityNever                          // skipped from the curated view
	VisibilityOptional                       // wallet may show or skip
	VisibilityIfNotIn                        // skipped only when value matches the deny-list
	VisibilityMustMatch                      // hidden but enforced — descriptor invalid if value not in allow-list
)

// Visibility is the polymorphic "visible" key on a field: either a
// simple string ("always" | "never" | "optional"), or an object
// carrying an ifNotIn / mustMatch rule.
type Visibility struct {
	Kind   VisibilityKind
	Values []string // for IfNotIn / MustMatch
}

// UnmarshalJSON tolerates either form.
func (v *Visibility) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		switch s {
		case "always", "":
			v.Kind = VisibilityAlways
		case "never":
			v.Kind = VisibilityNever
		case "optional":
			v.Kind = VisibilityOptional
		default:
			return fmt.Errorf("erc7730: unknown visibility %q", s)
		}
		return nil
	}
	if b[0] == '{' {
		obj := map[string][]string{}
		if err := json.Unmarshal(b, &obj); err != nil {
			return err
		}
		if vs, ok := obj["ifNotIn"]; ok {
			v.Kind = VisibilityIfNotIn
			v.Values = vs
			return nil
		}
		if vs, ok := obj["mustMatch"]; ok {
			v.Kind = VisibilityMustMatch
			v.Values = vs
			return nil
		}
	}
	return fmt.Errorf("erc7730: unrecognised visibility shape: %s", string(b))
}

// MarshalJSON is best-effort symmetric.
func (v Visibility) MarshalJSON() ([]byte, error) {
	switch v.Kind {
	case VisibilityAlways:
		return json.Marshal("always")
	case VisibilityNever:
		return json.Marshal("never")
	case VisibilityOptional:
		return json.Marshal("optional")
	case VisibilityIfNotIn:
		return json.Marshal(map[string][]string{"ifNotIn": v.Values})
	case VisibilityMustMatch:
		return json.Marshal(map[string][]string{"mustMatch": v.Values})
	}
	return []byte(`"always"`), nil
}

// IsVisibleByDefault reports whether a field with the zero Visibility
// (no explicit "visible" key in the JSON) should be shown.
func (v Visibility) IsVisibleByDefault() bool {
	return v.Kind == VisibilityAlways
}
