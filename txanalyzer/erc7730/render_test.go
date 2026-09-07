package erc7730

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tranvictor/jarvis/ui"
)

func sampleView() *ClearSignedView {
	return &ClearSignedView{
		InterpolatedIntent: "Swap 1,000 USDC for WETH",
		Owner:              "Uniswap",
		ContractName:       "Uniswap V2 Router",
		Source:             "registry",
		Fields: []FormattedField{
			{Label: "Amount in", Value: "1,000 USDC"},
		},
	}
}

func TestRenderIncludesSigningHint(t *testing.T) {
	var buf bytes.Buffer
	Render(ui.NewTerminalUIWithWriter(&buf, false), sampleView())
	out := buf.String()
	if !strings.Contains(out, signingHint) {
		t.Fatalf("signing render must include the hardware-wallet hint:\n%s", out)
	}
}

func TestRenderInfoOmitsSigningHint(t *testing.T) {
	var buf bytes.Buffer
	RenderInfo(ui.NewTerminalUIWithWriter(&buf, false), sampleView())
	out := buf.String()
	if !strings.Contains(out, "Clear Signed · Uniswap (Uniswap V2 Router)") {
		t.Fatalf("info render must still show the panel:\n%s", out)
	}
	if strings.Contains(out, signingHint) || strings.Contains(out, "hardware wallet") {
		t.Fatalf("info render must not include the hardware-wallet hint:\n%s", out)
	}
}

func TestRenderNilIsNoop(t *testing.T) {
	var buf bytes.Buffer
	Render(ui.NewTerminalUIWithWriter(&buf, false), nil)
	RenderInfo(ui.NewTerminalUIWithWriter(&buf, false), nil)
	if buf.Len() != 0 {
		t.Fatalf("nil view must print nothing: %q", buf.String())
	}
}
