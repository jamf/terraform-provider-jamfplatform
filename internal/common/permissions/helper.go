// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package permissions

import "strings"

// Helper describes the SDK calls a shared helper package makes on a construct's
// behalf, and the circumstance in which it makes them. A construct whose own
// source never names those methods still needs their privileges, and a
// permission table built from its own call list alone omits them.
//
// Every helper package that makes a hard-failing SDK call exports one Helper,
// and every construct importing that package renders it through SectionWith.
// The privileges are conditional, not required: a policy that names no directory
// group never searches LDAP, so ldap-servers:read is needed only when the
// configuration reaches the helper. A helper whose failures are swallowed
// (criteria's group-name lookups degrade to the wire value) is not declared.
type Helper struct {
	// When completes "Also grant these when …" and names the configuration that
	// reaches the helper, in the practitioner's vocabulary.
	When string
	// Registry is the SDK privilege registry holding Methods.
	Registry Registry
	// Methods are the SDK methods the helper calls.
	Methods []string
}

// SectionWith renders Section(reg, methods...) followed by one conditional
// table per helper. A capability the construct's own table already lists, with
// every action the helper needs, is left out of the helper's table, so a
// construct that already holds the privilege is not shown it twice. A helper
// with nothing left to add renders nothing.
func SectionWith(reg Registry, methods []string, helpers ...Helper) string {
	out := Section(reg, methods...)
	have := map[string]map[string]bool{}
	base := collect(reg, methods)
	for _, r := range append(append([]requirement{}, base.reqs...), base.alternatives...) {
		have[r.capability] = r.actions
	}

	var b strings.Builder
	for _, h := range helpers {
		c := collect(h.Registry, h.Methods)
		var rest []requirement
		for _, r := range c.reqs {
			if r.malformed {
				rest = append(rest, r)
				continue
			}
			missing := map[string]bool{}
			for a := range r.actions {
				if !have[r.capability][a] {
					missing[a] = true
				}
			}
			if len(missing) > 0 {
				r.actions = missing
				rest = append(rest, r)
			}
		}
		if len(rest) == 0 {
			continue
		}
		if b.Len() == 0 {
			b.WriteString("\n\n**Conditional permissions**\n\n")
		} else {
			b.WriteString("\n")
		}
		b.WriteString("Also grant these when ")
		b.WriteString(h.When)
		b.WriteString(":\n\n")
		writeRows(&b, rest)
	}
	if b.Len() == 0 {
		return out
	}
	return out + strings.TrimRight(b.String(), "\n")
}
