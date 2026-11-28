package holdings

import (
	"encoding/json"
	"math"
	"math/big"
	"os"
	"os/user"
	"path/filepath"
	"time"
)

const (
	priceMarkWindow = 24 * time.Hour
	priceMarkMinGap = 10 * time.Minute
	priceMarkMaxAge = 48 * time.Hour
)

// priceSample is one price we recorded for a holding.
type priceSample struct {
	At int64  `json:"t"`
	Px string `json:"p"`
}

// priceMarks is the on-disk history behind the 24h change.
//
// Ways this can fail, and what the code does about each:
//
//   - No sample is 24 hours old yet: the change stays unset. The arrow
//     slot is blank and the 24h column is a dash.
//   - The newest older sample is less than 24 hours old: it is not the
//     mark yet. The change stays unset.
//   - The mark or the current price is not a decimal, or the mark is zero:
//     the change stays unset.
//   - The move is under 0.05%: the percent is stored, and the arrow stays
//     off. That is the same cutoff as the 24h column.
//   - The history file is missing, unreadable, or not JSON: start empty.
//     The book still prints.
//   - The home directory cannot be created: marks stay in memory for this run.
//   - A sample older than 48 hours is dropped. A price from last week is
//     not a 24h mark.
//   - A live socket ticks constantly: a new sample is stored at most every
//     10 minutes. The comparison still uses the price on the row.
type priceMarks struct {
	path string
	rows map[string][]priceSample
}

func newPriceMarks(path string) *priceMarks {
	return &priceMarks{path: path, rows: map[string][]priceSample{}}
}

func priceMarkPath() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	if u.HomeDir == "" {
		return "", os.ErrNotExist
	}
	return filepath.Join(u.HomeDir, ".jarvis", "price-24h.json"), nil
}

func loadPriceMarks(path string) *priceMarks {
	m := newPriceMarks(path)
	if path == "" {
		return m
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return m
	}
	rows := map[string][]priceSample{}
	if json.Unmarshal(raw, &rows) != nil || rows == nil {
		return m
	}
	m.rows = rows
	return m
}

// openPriceMarks loads ~/.jarvis/price-24h.json once per book.
func (b *Book) openPriceMarks() {
	if b == nil || b.marks != nil {
		return
	}
	path, err := priceMarkPath()
	if err != nil {
		b.marks = newPriceMarks("")
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.marks = newPriceMarks("")
		return
	}
	b.marks = loadPriceMarks(path)
}

// notePrices records the price on each row and sets Change24h from the
// newest sample that is at least 24 hours older than now.
func (b *Book) notePrices(now time.Time) {
	if b == nil {
		return
	}
	if b.marks == nil {
		b.marks = newPriceMarks("")
	}
	wrote := false
	for _, p := range b.Positions {
		if b.marks.observe(p, now) {
			wrote = true
		}
	}
	if wrote {
		_ = b.marks.save()
	}
}

// observe stores p's current price when the last sample is old enough,
// then sets Change24h. It reports whether a sample was appended.
func (m *priceMarks) observe(p *Position, now time.Time) bool {
	if p == nil {
		return false
	}
	if p.Price == "" {
		p.Change24h = nil
		return false
	}
	key := p.Key()
	at := now.UnixMilli()
	rows := m.rows[key]
	wrote := false
	if len(rows) == 0 || at-rows[len(rows)-1].At >= priceMarkMinGap.Milliseconds() {
		if len(rows) == 0 || at >= rows[len(rows)-1].At {
			rows = append(rows, priceSample{At: at, Px: p.Price})
			wrote = true
		}
	}
	keep := now.Add(-priceMarkMaxAge).UnixMilli()
	pruned := rows[:0]
	for _, s := range rows {
		if s.At >= keep {
			pruned = append(pruned, s)
		}
	}
	rows = pruned
	m.rows[key] = rows
	p.Change24h = changeVsMark(p.Price, rows, now.Add(-priceMarkWindow).UnixMilli())
	return wrote
}

func changeVsMark(current string, rows []priceSample, cutoff int64) *float64 {
	var mark string
	found := false
	for _, s := range rows {
		if s.At <= cutoff {
			mark = s.Px
			found = true
		}
	}
	if !found {
		return nil
	}
	pct, ok := percentChange(current, mark)
	if !ok {
		return nil
	}
	return &pct
}

func percentChange(current, mark string) (float64, bool) {
	cur, ok1 := new(big.Rat).SetString(current)
	old, ok2 := new(big.Rat).SetString(mark)
	if !ok1 || !ok2 || old.Sign() == 0 {
		return 0, false
	}
	diff := new(big.Rat).Sub(cur, old)
	diff.Quo(diff, old)
	diff.Mul(diff, big.NewRat(100, 1))
	f, _ := diff.Float64()
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func (m *priceMarks) save() error {
	if m == nil || m.path == "" {
		return nil
	}
	raw, err := json.Marshal(m.rows)
	if err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}
