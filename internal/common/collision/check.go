// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package collision

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Request describes one planned key set for one namespace.
//
// Keys are the planned values and PriorKeys the values in state, nil on a create.
// A key is checked when it is new, that is on a create or when it differs from
// every prior key under the namespace's collation. SelfID is the planned
// object's own ID, empty on a create, and ResourceType the Terraform type being
// planned. Together they keep an object from colliding with itself.
type Request struct {
	Namespace    Namespace
	ResourceType string
	Path         path.Path
	Keys         []string
	PriorKeys    []string
	SelfID       string
}

// Check reports every planned key that collides with an existing object. It
// returns nothing when the checker is nil or the namespace is not covered by the
// configuration.
func (c *Checker) Check(ctx context.Context, req Request) diag.Diagnostics {
	var diags diag.Diagnostics
	if !c.applies(req.Namespace) {
		return diags
	}

	fresh := newKeys(req)
	if len(fresh) == 0 {
		return diags
	}

	s := c.load(ctx, req.Namespace)
	if s.err != nil {
		s.notice.Do(func() {
			diags.AddWarning(
				"Name collision check unavailable",
				fmt.Sprintf("The provider could not list the existing %ss, so it could not check whether this plan reuses a %s that is already taken: %s. The plan continues without the check.",
					req.Namespace.Sources[0].Noun, req.Namespace.keyLabel(), s.err),
			)
		})
		return diags
	}

	for _, key := range fresh {
		for _, m := range s.find(req.Namespace, key, req.SelfID, req.ResourceType) {
			c.report(&diags, req, key, m)
		}
	}
	return diags
}

func newKeys(req Request) []string {
	var out []string
	for _, key := range req.Keys {
		if key == "" {
			continue
		}
		seen := false
		for _, prior := range req.PriorKeys {
			if req.Namespace.Collation.Equal(key, prior) {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, key)
		}
	}
	return out
}

func (c *Checker) report(diags *diag.Diagnostics, req Request, key string, m Match) {
	summary, detail := render(req, key, m)
	if c.cfg.Mode == ModeWarn {
		diags.AddAttributeWarning(req.Path, summary, detail)
		return
	}
	diags.AddAttributeError(req.Path, summary, detail)
}

func render(req Request, key string, m Match) (summary, detail string) {
	ns := req.Namespace
	label := ns.keyLabel()
	article := "A"
	if strings.ContainsRune("aeiouAEIOU", rune(m.Noun[0])) {
		article = "An"
	}

	var b strings.Builder
	if ns.Enforced {
		summary = fmt.Sprintf("%s already in use", capitalise(label))
		fmt.Fprintf(&b, "%s %s with the %s %q already exists in %s (ID %s), and %s does not allow two with the same %s. %s",
			article, m.Noun, label, key, ns.Product, m.ID, ns.Product, label, ns.Collation.Describe(label))
	} else {
		summary = fmt.Sprintf("%s already in use (duplicate check)", capitalise(label))
		fmt.Fprintf(&b, "%s %s with the %s %q already exists in %s (ID %s). %s accepts duplicates, so the apply would succeed and leave two. This check is on because include_unenforced is set. %s",
			article, m.Noun, label, key, ns.Product, m.ID, ns.Product, ns.Collation.Describe(label))
	}

	if m.ResourceType != "" && m.ResourceType == req.ResourceType {
		fmt.Fprintf(&b, "\n\nTo manage the existing %s with Terraform, import it:\n\n  import {\n    to = <this resource's address>\n    id = %q\n  }\n\nOtherwise, choose a different %s. If this configuration already manages the existing %s under a different address, add a moved block instead.",
			m.Noun, m.ID, label, m.Noun)
	} else if m.ResourceType != "" {
		fmt.Fprintf(&b, "\n\nThe existing %s is managed by %s, not by this resource. Choose a different %s, or import it into a %s resource.",
			m.Noun, m.ResourceType, label, m.ResourceType)
	} else {
		fmt.Fprintf(&b, "\n\nChoose a different %s.", label)
	}
	return summary, b.String()
}

func capitalise(s string) string {
	return strings.ToUpper(s[:1]) + s[1:]
}

// PlanReport configures ReportPlan for one resource.
//
// Each rule checks one namespace against the key values its extractor returns,
// so a package with one uniqueness rule passes one rule and a package with two
// (a package's name and file name) passes two. IDPath locates the resource's own
// ID and defaults to the top-level "id" attribute.
type PlanReport[M any] struct {
	Checker      *Checker
	ResourceType string
	IDPath       path.Path
	Rules        []Rule[M]
}

// Rule pairs a namespace with the attribute that carries its key.
type Rule[M any] struct {
	Namespace Namespace
	Path      path.Path
	Keys      func(*M) []types.String
}

// ReportPlan is the shared ModifyPlan hook. It decodes prior state and the
// plan, skips destroys and unknown keys, and runs each rule.
//
// An unknown key is skipped without a note: ModifyPlan runs again during apply
// with the value resolved, which is the check that counts for it.
func ReportPlan[M any](
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
	rep PlanReport[M],
) {
	if !rep.Checker.Enabled() || req.Plan.Raw.IsNull() {
		return
	}

	var plan M
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		return
	}

	var (
		prior  M
		hasOld = !req.State.Raw.IsNull()
		selfID string
	)
	if hasOld {
		if diags := req.State.Get(ctx, &prior); diags.HasError() {
			return
		}
		idPath := rep.IDPath
		if idPath.Equal(path.Empty()) {
			idPath = path.Root("id")
		}
		var id types.String
		if diags := req.State.GetAttribute(ctx, idPath, &id); !diags.HasError() {
			selfID = id.ValueString()
		}
	}

	for _, rule := range rep.Rules {
		keys, known := knownValues(rule.Keys(&plan))
		if !known || len(keys) == 0 {
			continue
		}
		var priorKeys []string
		if hasOld {
			priorKeys, _ = knownValues(rule.Keys(&prior))
		}
		resp.Diagnostics.Append(rep.Checker.Check(ctx, Request{
			Namespace:    rule.Namespace,
			ResourceType: rep.ResourceType,
			Path:         rule.Path,
			Keys:         keys,
			PriorKeys:    priorKeys,
			SelfID:       selfID,
		})...)
	}
}

func knownValues(values []types.String) (out []string, allKnown bool) {
	allKnown = true
	for _, v := range values {
		switch {
		case v.IsUnknown():
			allKnown = false
		case v.IsNull():
		default:
			out = append(out, v.ValueString())
		}
	}
	return out, allKnown
}

// SetKeys flattens a set of strings into key values for a Rule. A null set has no
// keys, and an unknown set yields one unknown value so ReportPlan skips the rule
// until apply resolves it.
func SetKeys(set types.Set) []types.String {
	switch {
	case set.IsNull():
		return nil
	case set.IsUnknown():
		return []types.String{types.StringUnknown()}
	}
	elements := set.Elements()
	out := make([]types.String, 0, len(elements))
	for _, el := range elements {
		if s, ok := el.(types.String); ok {
			out = append(out, s)
		}
	}
	return out
}
