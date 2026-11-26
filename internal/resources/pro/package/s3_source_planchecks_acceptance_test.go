// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package pkg_test

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// s3PlanPath addresses a top-level attribute in a plan check.
func s3PlanPath(attr string) tfjsonpath.Path {
	return tfjsonpath.New(attr)
}

// s3LazyString is a knownvalue.Check against a string that an earlier step
// decides, so it is read when the check runs and not when the step is built.
type s3LazyString struct {
	want func() string
}

// s3StringExact returns a check that the planned value is the string want
// returns at the time of the check.
func s3StringExact(want func() string) knownvalue.Check {
	return s3LazyString{want: want}
}

// CheckValue implements knownvalue.Check.
func (l s3LazyString) CheckValue(got any) error {
	text, ok := got.(string)
	if !ok {
		return fmt.Errorf("expected a string, got %T", got)
	}
	if want := l.want(); text != want {
		return fmt.Errorf("expected %q, got %q", want, text)
	}
	return nil
}

// String implements knownvalue.Check.
func (l s3LazyString) String() string {
	return l.want()
}
