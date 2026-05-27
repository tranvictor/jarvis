package erc7730

import "testing"

func TestMergeOverlay(t *testing.T) {
	base := &Descriptor{
		Metadata: Metadata{Owner: "BaseOwner", ContractName: "Base"},
		Display: Display{Formats: map[string]*Format{
			"a(uint256)": {
				Intent: Intent{Text: "BaseA"},
				Fields: []Field{{Path: "v", Label: "Value"}},
			},
		}},
	}
	overlay := &Descriptor{
		Metadata: Metadata{ContractName: "Override"},
		Display: Display{Formats: map[string]*Format{
			"a(uint256)": {
				Intent: Intent{Text: "OverlayA"},
				Fields: []Field{{Path: "v", Label: "Value (overlay)"}},
			},
			"b(address)": {Intent: Intent{Text: "OverlayB"}},
		}},
	}
	merged := Merge(base, overlay)
	if merged.Metadata.Owner != "BaseOwner" {
		t.Errorf("owner: want BaseOwner got %q", merged.Metadata.Owner)
	}
	if merged.Metadata.ContractName != "Override" {
		t.Errorf("contractName: want Override got %q", merged.Metadata.ContractName)
	}
	if len(merged.Display.Formats) != 2 {
		t.Errorf("formats: want 2 got %d", len(merged.Display.Formats))
	}
	if got := merged.Display.Formats["a(uint256)"].Intent.Text; got != "OverlayA" {
		t.Errorf("overlay intent: got %q", got)
	}
	if got := merged.Display.Formats["a(uint256)"].Fields[0].Label; got != "Value (overlay)" {
		t.Errorf("overlay field: got %q", got)
	}
}
