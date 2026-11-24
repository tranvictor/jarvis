package holdings

import "sort"

func sortStrings(s []string) { sort.Strings(s) }

func sortInts(s []int) { sort.Ints(s) }

func sortPositions(ps []*Position, chainTotal map[int]float64) {
	sort.SliceStable(ps, func(i, j int) bool {
		a, c := ps[i], ps[j]
		if chainTotal[a.ChainID] != chainTotal[c.ChainID] {
			return chainTotal[a.ChainID] > chainTotal[c.ChainID]
		}
		if a.ChainID != c.ChainID {
			return a.ChainName < c.ChainName
		}
		av, cv := -1.0, -1.0
		if a.Value != nil {
			av = *a.Value
		}
		if c.Value != nil {
			cv = *c.Value
		}
		if av != cv {
			return av > cv
		}
		return displaySymbol(a) < displaySymbol(c)
	})
}
