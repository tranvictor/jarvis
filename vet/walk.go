package vet

import (
	"strings"

	jarviscommon "github.com/tranvictor/jarvis/common"
)

func walkCalls(fc *jarviscommon.FunctionCall, fn func(*jarviscommon.FunctionCall)) {
	if fc == nil {
		return
	}
	fn(fc)
	for _, inner := range fc.DecodedFunctionCalls {
		walkCalls(inner, fn)
	}
}

func walkAddresses(fc *jarviscommon.FunctionCall, fn func(jarviscommon.Address)) {
	walkCalls(fc, func(c *jarviscommon.FunctionCall) {
		if c.Destination.Address != "" {
			fn(c.Destination)
		}
		walkParams(c.Params, fn)
	})
}

func walkParams(ps []jarviscommon.ParamResult, fn func(jarviscommon.Address)) {
	for _, p := range ps {
		for _, v := range p.Values {
			if v.Address != nil && v.Address.Address != "" {
				fn(*v.Address)
			}
		}
		for _, t := range p.Tuples {
			walkParams(t.Values, fn)
		}
		walkParams(p.Arrays, fn)
	}
}

func paramValue(fc *jarviscommon.FunctionCall, names ...string) *jarviscommon.Value {
	if fc == nil {
		return nil
	}
	want := map[string]struct{}{}
	for _, n := range names {
		want[strings.ToLower(n)] = struct{}{}
	}
	for i := range fc.Params {
		p := &fc.Params[i]
		if _, ok := want[strings.ToLower(p.Name)]; !ok {
			continue
		}
		if len(p.Values) == 1 {
			return &p.Values[0]
		}
	}
	return nil
}

func paramAddress(fc *jarviscommon.FunctionCall, names ...string) *jarviscommon.Address {
	v := paramValue(fc, names...)
	if v == nil || v.Address == nil {
		return nil
	}
	return v.Address
}
