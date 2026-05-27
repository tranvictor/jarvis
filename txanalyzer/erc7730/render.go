package erc7730

import (
	"fmt"
	"strings"

	"github.com/tranvictor/jarvis/ui"
)

// Render writes view inside a green-bordered BoxedSection followed by
// a green-bordered fields table — the standard jarvis "clear signed"
// presentation. Callers wrap their existing per-tx printing with a
// Render() call when the engine returns a non-nil view.
//
// Render is a no-op when view is nil so call sites can write:
//
//	if v, _ := engine.ContractView(ctx, …); v != nil {
//	    erc7730.Render(u, v)
//	}
func Render(u ui.UI, view *ClearSignedView) {
	if view == nil {
		return
	}

	// Box header carries owner / contractName when present so the
	// title becomes "Clear Signed · Uniswap V3 Router" rather than
	// the bare protocol name.
	title := "Clear Signed · ERC-7730"
	switch {
	case view.ContractName != "" && view.Owner != "" &&
		!strings.EqualFold(view.ContractName, view.Owner):
		title = fmt.Sprintf("Clear Signed · %s (%s)", view.Owner, view.ContractName)
	case view.Owner != "":
		title = "Clear Signed · " + view.Owner
	case view.ContractName != "":
		title = "Clear Signed · " + view.ContractName
	}

	u.BoxedSection(ui.SeveritySuccess, title, func(c ui.UI) {
		if view.InterpolatedIntent != "" {
			c.Info(view.InterpolatedIntent)
		}
		for _, line := range view.Intent {
			c.Info(line)
		}
		c.Info("")
		c.Info(provenanceLine(view))
		c.Info("Compare with your hardware wallet screen before approving.")
		if view.Warning != "" {
			c.Warn(view.Warning)
		}
	})

	if len(view.Fields) == 0 {
		return
	}

	rows := make([][]ui.TableCell, 0, len(view.Fields))
	for _, f := range view.Fields {
		sev := ui.SeverityInfo
		if f.Warn {
			sev = ui.SeverityWarn
		}
		rows = append(rows, []ui.TableCell{
			ui.TC(f.Label),
			ui.TCS(f.Value, sev),
		})
	}
	u.PrintTable(&ui.Table{
		Headers:        []string{"Field", "Value"},
		Rows:           rows,
		BorderSeverity: ui.SeveritySuccess,
	})
}

func provenanceLine(v *ClearSignedView) string {
	source := v.Source
	if source == "" {
		source = "unknown"
	}
	cached := ""
	if !v.CachedAt.IsZero() {
		cached = " · cached " + v.CachedAt.Format("2006-01-02")
	}
	return "Source: ERC-7730 " + source + cached
}
