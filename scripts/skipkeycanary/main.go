//go:build skipkeycanary

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Command skipkeycanary detects Setup Assistant panes that Jamf Pro has added to or removed from
// its PreStage skip list, which the OpenAPI spec and the SDK cannot show.
//
// The spec types `skipSetupItems` as a map of booleans with no enumeration, so a pane Jamf adds
// (Liquid Glass, Device Features Tour) is invisible until a person notices it missing in the admin
// UI. A PreStage created with no skip items set is still returned with every key Jamf Pro
// supports, which is the only machine-readable list there is. The canary creates a throwaway
// PreStage of each family, reads the keys back, compares them with the tables the two PreStage
// resources manage, and files a GitHub issue for any difference. It never opens a pull request:
// the API names a pane by wire key and not by the label the admin UI shows, so a person has to
// supply the attribute name from a screenshot.
//
// It makes no assumption about the tenant. It uploads the Automated Device Enrollment token itself
// and creates and deletes its own PreStages, so it works on an empty tenant. If the tenant already
// holds the token, it reuses that instance and leaves it alone, but only when exactly one
// instance matches JAMFPLATFORM_ACC_PRO_DEP_SERVER_UUID (optional) or the canary's own instance
// name; with no match, or several, the run fails rather than guess. Only objects named with the
// zz-canary- prefix, or created during the run, are ever deleted. Each run first sweeps what a
// crashed run left behind, and ends by listing the tenant again and failing if any canary object
// remains. SIGINT and SIGTERM cancel the run and the cleanup still runs; SIGKILL cannot be caught.
//
// Drift is not a failure. Authentication, upload and cleanup errors are, and exit with status 1.
//
// Credentials come from the provider's own variables: JAMFPLATFORM_BASE_URL,
// JAMFPLATFORM_CLIENT_ID and JAMFPLATFORM_CLIENT_SECRET, with at most one of
// JAMFPLATFORM_ENVIRONMENT_ID or JAMFPLATFORM_TENANT_ID. The token is JAMFPLATFORM_ACC_PRO_DEP_TOKEN,
// the base64 of the whole .p7m file.
//
// Usage:
//
//	go run -tags skipkeycanary ./scripts/skipkeycanary -dry-run
//	go run -tags skipkeycanary ./scripts/skipkeycanary -no-issues
//	go run -tags skipkeycanary ./scripts/skipkeycanary
//
// The build tag keeps the tool out of `go build ./...` while `go test -tags skipkeycanary` still
// exercises it, as for scripts/acclanes.
package main

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/computer_prestage_enrollment"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/mobile_device_prestage_enrollment"
)

const (
	tokenVariable      = "JAMFPLATFORM_ACC_PRO_DEP_TOKEN"
	serverUUIDVariable = "JAMFPLATFORM_ACC_PRO_DEP_SERVER_UUID"
	appleTitlesURL     = "https://raw.githubusercontent.com/apple/device-management/release/other/skipkeys.yaml"
)

// notifyContext and connectAPI are variables so a test can drive a whole run without a signal or a
// network: the first stands in for the operating system delivering SIGINT or SIGTERM, the second
// for authentication against Jamf Pro.
var (
	notifyContext = signal.NotifyContext
	connectAPI    = connect
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("skipkeycanary", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dryRun := fs.Bool("dry-run", false, "list what would be swept, created and deleted; change nothing")
	noIssues := fs.Bool("no-issues", false, "report drift on stdout and do not file GitHub issues")
	timeout := fs.Duration("timeout", 20*time.Minute, "overall time limit for the run")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	logf := func(format string, a ...any) { _, _ = fmt.Fprintf(stderr, format+"\n", a...) }

	api, token, err := connectAPI(getenv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "skipkeycanary:", err)
		return 1
	}

	sigCtx, stop := notifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(sigCtx, *timeout)
	defer cancel()

	outcome, runErr := Run(ctx, api, Options{
		DryRun:         *dryRun,
		Token:          token,
		InstanceName:   NamePrefix + "ade",
		ServerUUID:     strings.TrimSpace(getenv(serverUUIDVariable)),
		CleanupTimeout: 5 * time.Minute,
		Logf:           logf,
	})
	if runErr != nil {
		_, _ = fmt.Fprintln(stderr, "skipkeycanary:", runErr)
		return 1
	}
	if *dryRun {
		return 0
	}

	known := map[Kind][]string{
		Computer: computer_prestage_enrollment.SkipSetupWireKeys(),
		Mobile:   mobile_device_prestage_enrollment.SkipSetupWireKeys(),
	}
	drift := Collect(outcome.Observed, known)
	report(stdout, outcome, known, drift)

	if *noIssues || (len(drift[New]) == 0 && len(drift[Removed]) == 0) {
		return 0
	}
	apple, appleErr := fetchAppleTitles(ctx)
	if appleErr != nil {
		logf("Apple's titles unavailable, filing without them: %v", appleErr)
	}
	if err := Reconcile(ctx, ghTracker{}, drift, apple, logf); err != nil {
		_, _ = fmt.Fprintln(stderr, "skipkeycanary:", err)
		return 1
	}
	return 0
}

func connect(getenv func(string) string) (API, []byte, error) {
	baseURL := getenv("JAMFPLATFORM_BASE_URL")
	clientID := getenv("JAMFPLATFORM_CLIENT_ID")
	clientSecret := getenv("JAMFPLATFORM_CLIENT_SECRET")
	if baseURL == "" || clientID == "" || clientSecret == "" {
		return nil, nil, errors.New("set JAMFPLATFORM_BASE_URL, JAMFPLATFORM_CLIENT_ID and JAMFPLATFORM_CLIENT_SECRET")
	}
	raw := strings.Join(strings.Fields(getenv(tokenVariable)), "")
	if raw == "" {
		return nil, nil, fmt.Errorf("%s is not set; the canary needs a token to upload", tokenVariable)
	}
	token, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("%s is not valid base64: %w", tokenVariable, err)
	}

	env, tenant := getenv("JAMFPLATFORM_ENVIRONMENT_ID"), getenv("JAMFPLATFORM_TENANT_ID")
	var opts []jamfplatform.Option
	switch {
	case env != "" && tenant != "":
		return nil, nil, errors.New("JAMFPLATFORM_ENVIRONMENT_ID and JAMFPLATFORM_TENANT_ID are both set; unset one")
	case env != "":
		opts = append(opts, jamfplatform.WithEnvironmentID(env))
	case tenant != "":
		opts = append(opts, jamfplatform.WithTenantID(tenant))
	}
	client := jamfplatform.NewClient(baseURL, clientID, clientSecret, opts...)
	if err := client.ValidateCredentials(context.Background()); err != nil {
		return nil, nil, fmt.Errorf("credentials rejected: %w", err)
	}
	return live{pro: pro.New(client)}, token, nil
}

func fetchAppleTitles(ctx context.Context) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, appleTitlesURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", appleTitlesURL, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	return ParseAppleTitles(string(body)), nil
}

func report(w io.Writer, outcome Outcome, known map[Kind][]string, drift map[Direction][]Drift) {
	for _, kind := range Kinds {
		_, _ = fmt.Fprintf(w, "%s: %d keys on the wire, %d in the provider\n", kind, len(outcome.Observed[kind]), len(known[kind]))
	}
	for _, dir := range []Direction{New, Removed} {
		if len(drift[dir]) == 0 {
			_, _ = fmt.Fprintf(w, "no %s keys\n", dir)
			continue
		}
		_, _ = fmt.Fprintf(w, "%s keys:\n", dir)
		for _, d := range drift[dir] {
			_, _ = fmt.Fprintf(w, "  %s %s\n", d.Kind, d.Key)
		}
	}
}
