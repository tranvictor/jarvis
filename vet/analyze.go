package vet

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tranvictor/jarvis/vet/ai"
)

// Analyze runs the selected measures. ModeAlways is DELEGATECALL only.
// ModeFull runs every local measure and Grok when a Completer is set.
func Analyze(ctx context.Context, req Request) Report {
	var findings []Finding
	var skipped []string

	findings = append(findings, measureDelegateCall(req)...)
	if req.Mode != ModeFull {
		return Report{Findings: findings}
	}

	findings = append(findings, measureCreate(req)...)
	findings = append(findings, measureUnverified(req)...)
	findings = append(findings, measureMethods(req)...)
	findings = append(findings, measurePoison(req)...)

	if req.Create {
		return Report{Findings: findings, Skipped: skipped}
	}

	grokFindings, skip := runGrok(ctx, req, findings)
	if skip != "" {
		skipped = append(skipped, skip)
		findings = append(findings, Finding{
			Code: CodeAISkip,
			Risk: RiskCaution,
			Text: "AI review skipped: " + skip,
		})
	}
	findings = append(findings, grokFindings...)
	return Report{Findings: findings, Skipped: skipped}
}

// AnalyzeTypedData reviews an eth_signTypedData_v4 Permit (and related) message.
func AnalyzeTypedData(ctx context.Context, req TypedRequest) Report {
	if req.Mode != ModeFull {
		return Report{}
	}
	findings := measureTypedData(req)
	src := Source{}
	if req.Chain != nil && req.Verifying != "" {
		if s, err := req.Chain.Source(req.Verifying); err == nil {
			src = s
		}
	}
	if !src.Verified || src.Code == "" {
		if req.Verifying != "" {
			findings = append(findings, Finding{
				Code: CodeUnverified,
				Risk: RiskDanger,
				Text: fmt.Sprintf("%s has no verified source; vet cannot read the code that will run", req.Verifying),
			})
		}
		return Report{Findings: findings}
	}
	grokFindings, skip := runGrokTyped(ctx, req, src, findings)
	if skip != "" {
		findings = append(findings, Finding{
			Code: CodeAISkip,
			Risk: RiskCaution,
			Text: "AI review skipped: " + skip,
		})
	}
	findings = append(findings, grokFindings...)
	return Report{Findings: findings}
}

func runGrok(ctx context.Context, req Request, local []Finding) ([]Finding, string) {
	if hasCode(local, CodeUnverified) || hasCode(local, CodeProxyImplUnverified) || hasCode(local, CodeCreate) {
		return nil, ""
	}
	if req.AI == nil {
		return nil, "no AI client"
	}
	src := Source{}
	dest := effectiveDest(req)
	if req.Chain != nil && dest != "" {
		s, err := req.Chain.Source(dest)
		if err != nil {
			return nil, err.Error()
		}
		src = s
		if s.Implementation != "" && !sameAddr(s.Implementation, dest) {
			if impl, err := req.Chain.Source(s.Implementation); err == nil && impl.Code != "" {
				src = impl
			}
		}
	}
	if src.Code == "" {
		return nil, ""
	}
	p, ok := ToPayload(req, src, codesOf(local))
	if !ok {
		return nil, "unclassified value in call"
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err.Error()
	}
	reply, err := req.AI.Complete(ctx, raw)
	if err != nil {
		return nil, err.Error()
	}
	return applyReply(reply, local), ""
}

func runGrokTyped(ctx context.Context, req TypedRequest, src Source, local []Finding) ([]Finding, string) {
	if req.AI == nil {
		return nil, "no AI client"
	}
	p, ok := typedPayload(req, src, codesOf(local))
	if !ok {
		return nil, "unclassified value in typed data"
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err.Error()
	}
	reply, err := req.AI.Complete(ctx, raw)
	if err != nil {
		return nil, err.Error()
	}
	return applyReply(reply, local), ""
}

func applyReply(reply ModelReply, local []Finding) []Finding {
	allowed := map[string]struct{}{}
	for _, c := range codesOf(local) {
		allowed[c] = struct{}{}
	}
	reconfirm := map[string]struct{}{}
	for _, c := range reply.Reconfirms {
		if _, ok := allowed[c]; ok {
			reconfirm[c] = struct{}{}
		}
	}
	for i := range local {
		if _, ok := reconfirm[local[i].Code]; ok {
			local[i].GrokReconfirm = true
		}
	}
	var out []Finding
	risk := RiskCaution
	if reply.Risk == "danger" {
		risk = RiskDanger
	}
	for _, b := range reply.Bullets {
		if b == "" {
			continue
		}
		out = append(out, Finding{
			Code: CodeGrok,
			Risk: risk,
			Text: "Grok: " + b,
		})
	}
	if reply.AssetEffect != "" {
		out = append(out, Finding{
			Code: CodeGrok,
			Risk: risk,
			Text: "Grok: " + reply.AssetEffect,
		})
	}
	return out
}

func codesOf(fs []Finding) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, f := range fs {
		if f.Code == "" || f.Code == CodeGrok || f.Code == CodeAISkip {
			continue
		}
		if _, ok := seen[f.Code]; ok {
			continue
		}
		seen[f.Code] = struct{}{}
		out = append(out, f.Code)
	}
	return out
}

func hasCode(fs []Finding, code string) bool {
	for _, f := range fs {
		if f.Code == code {
			return true
		}
	}
	return false
}

// AdaptReply maps the ai client reply onto ModelReply.
func AdaptReply(r ai.Reply) ModelReply {
	return ModelReply{
		Risk:        r.Risk,
		Bullets:     r.Bullets,
		AssetEffect: r.AssetEffect,
		Reconfirms:  r.Reconfirms,
	}
}

// EnvCompleter wraps [ai.Client] so Analyze can call it.
type EnvCompleter struct {
	Client *ai.Client
}

func (e EnvCompleter) Complete(ctx context.Context, payload []byte) (ModelReply, error) {
	if e.Client == nil {
		return ModelReply{}, fmt.Errorf("XAI_API_KEY is not set")
	}
	if e.Client.Key == "" {
		return ModelReply{}, fmt.Errorf("XAI_API_KEY is not set")
	}
	r, err := e.Client.Complete(ctx, payload)
	if err != nil {
		return ModelReply{}, err
	}
	return AdaptReply(r), nil
}
