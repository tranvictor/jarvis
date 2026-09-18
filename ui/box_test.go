package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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

func TestBoxedSectionWrapsLongAddressName(t *testing.T) {
	long := "0x2515ec2104d30a073c0ea99d916b9e20b14fc7e0 (Lumen Main Multisig (Mike, Victor, Loi) – Eth, Bsc, Arb, Opt, Base, Pol, Robin, Avax, Bera, S, Plasma, Monad)"
	var buf strings.Builder
	u := NewTerminalUIWithWriter(&buf, false)
	u.width = 72
	u.BoxedSection(SeverityCritical, "Classic multisig transaction", func(c UI) {
		c.Subsection("Send  38,000 USDC  from  " + long + "  →  0xa230dfde6fadf03feff9cc95b73cdeba5eec8375")
		c.KeyValueCells([][2]TableCell{
			{MutedCell("Calls"), TC("0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48 (USDC)")},
			{MutedCell("Multisig"), TCS(long, SeveritySuccess)},
		})
	})
	out := buf.String()
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "Lumen Main Multisig") {
		t.Fatalf("name must still be visible:\n%s", plain)
	}
	var boxWidth int
	for _, line := range strings.Split(strings.TrimRight(plain, "\n"), "\n") {
		w := VisibleWidth(line)
		if strings.HasPrefix(strings.TrimLeft(line, " "), "╭") {
			boxWidth = w
		}
		if w > 72 {
			t.Fatalf("line is %d cols, want ≤ 72:\n%q", w, line)
		}
	}
	if boxWidth == 0 {
		t.Fatalf("missing box top border:\n%s", plain)
	}
	for _, line := range strings.Split(strings.TrimRight(plain, "\n"), "\n") {
		if strings.Contains(line, "╭") || strings.Contains(line, "╰") || strings.Contains(line, "│") {
			if VisibleWidth(line) != boxWidth {
				t.Fatalf("box line width %d, top is %d:\n%q\n%s", VisibleWidth(line), boxWidth, line, plain)
			}
		}
	}
}
