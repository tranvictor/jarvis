package reader

import "testing"

func TestEffectiveGasTipGwei(t *testing.T) {
	const eps = 1e-9
	cases := []struct {
		name            string
		node, max, base float64
		want            float64
	}{
		{
			name: "node oracle at 0, maxFee is only baseFee headroom",
			// eth_gasPrice = 20, maxFee = 30, baseFee = 20 → market tip 0
			// L1 floor 1 gwei so we don't broadcast tip 0
			node: 0, max: 30, base: 20, want: 1,
		},
		{
			name: "implied market tip from eth_gasPrice - baseFee",
			// eth_gasPrice = 20, maxFee = 30, baseFee = 10 → market 10
			node: 0, max: 30, base: 10, want: 10,
		},
		{
			name: "node tip higher than implied market",
			node: 12, max: 30, base: 10, want: 12,
		},
		{
			name: "L2-like tiny base fee uses 0.01 floor not 1 gwei",
			node: 0, max: 0.045, base: 0.03, want: 0.01,
		},
		{
			name: "L2 implied tip above the floor",
			// eth_gasPrice = 0.03, maxFee = 0.045, baseFee = 0.01 → 0.02
			node: 0, max: 0.045, base: 0.01, want: 0.02,
		},
		{
			name: "never exceed maxFee",
			node: 50, max: 8, base: 20, want: 8,
		},
		{
			name: "maxFee 0 still returns the floor when baseFee looks like L1",
			node: 0, max: 0, base: 15, want: 1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := EffectiveGasTipGwei(c.node, c.max, c.base)
			if diff := got - c.want; diff > eps || diff < -eps {
				t.Fatalf("EffectiveGasTipGwei(%v, %v, %v) = %v, want %v",
					c.node, c.max, c.base, got, c.want)
			}
		})
	}
}
