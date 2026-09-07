package ui

import (
	"strings"
	"testing"
)

func TestBoxedSectionFitsTitleWhenBodyIsShort(t *testing.T) {
	var buf strings.Builder
	u := NewTerminalUIWithWriter(&buf, false)
	u.BoxedSection(SeveritySuccess, "Clear Signed · Uniswap (Uniswap V2 Router)", func(c UI) {
		c.Info("Swap 1,000 USDC for WETH")
		c.Info("")
		c.Info("Source: ERC-7730 registry")
	})
	out := buf.String()
	if !strings.Contains(out, "Clear Signed · Uniswap (Uniswap V2 Router)") {
		t.Fatalf("title must not be truncated to fit a short body:\n%s", out)
	}
	if strings.Contains(out, "hardware wallet") {
		t.Fatalf("fixture should not include a signing hint:\n%s", out)
	}
}
