// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package filesources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	awsbase "github.com/hashicorp/aws-sdk-go-base/v2"
	awsbasediag "github.com/hashicorp/aws-sdk-go-base/v2/diag"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// markerETag is the kind prefix of a marker that holds an object's ETag.
//
// The ETag is the marker on both stores, on a versioned bucket too. Probed
// against real buckets on 2026-11-22: an S3 ETag changes when the content
// does and not on an identical re-upload, a metadata or storage-class copy in
// place, or a tag change, where VersionId moves on every write. It is never
// read as a digest: a multipart or SSE-KMS ETag is not an MD5.
const markerETag = "etag:"

// GuideURL is the registry guide for cloud storage sources, which the
// credential module's own messages point the reader to.
const GuideURL = "https://registry.terraform.io/providers/jamf/jamfplatform/latest/docs/guides/package-cloud-storage"

// fallbackRegion is the Region a client starts in when configuration, the
// environment and the shared files name none. Any Region will do, because a
// bucket elsewhere is found from the first response; this is the one whose
// endpoint has always existed.
const fallbackRegion = "us-east-1"

// bucketRegionHeader is the response header S3 names a bucket's Region in. It
// is on the 301 a wrong-Region request draws and on the answer to an
// unauthenticated request for the bucket.
const bucketRegionHeader = "x-amz-bucket-region"

// buildFailureHold is how long a failed client build is reported to later
// callers without being repeated. Long enough to cover the callers queued
// behind it, short enough that the next plan tries again. A variable so tests
// can shorten it.
var buildFailureHold = 15 * time.Second

// regionLookupTimeout bounds the unauthenticated request that finds a
// bucket's Region.
const regionLookupTimeout = 15 * time.Second

// s3Driver reads s3:// sources.
//
// The AWS configuration is built on first use and once. Building it resolves
// credentials, which for a role is a call to AWS STS, and by default validates
// them with a second one; many packages plan concurrently, and most
// configurations name no S3 source at all. A failed build is not kept, so a
// transient failure on the first package does not fail every later one. It is
// held for buildFailureHold, though: callers queue on the lock while a build
// runs, and without the hold each would repeat a slow failing build in turn,
// spending its own deadline in the queue. A build cut short by its caller's
// context is not held and not called a credential failure, because it says
// nothing about the credentials.
//
// Credential validation is skipped when file_sources.s3.endpoints.s3 is set
// and endpoints.sts is not. The validation call goes to AWS STS, and a store
// that is not AWS (MinIO, Cloudflare R2) has keys AWS has never heard of. The
// first request to the store reports a bad key there.
//
// A bucket's Region is discovered, not configured. A request sent to the
// wrong Region is answered 301 and the SDK does not follow it (probed
// 2026-11-22, three times out of three). The answer names the right Region in
// a header; when it does not, an unauthenticated request for the bucket does,
// which needs no permission. Each bucket's Region is kept for the life of the
// driver.
type s3Driver struct {
	config       S3Config
	userAgent    string
	lookupRegion func(ctx context.Context, bucket string) (string, error)

	mu          sync.Mutex
	base        *aws.Config
	buildErr    error
	buildFailed time.Time
	regions     map[string]string
	clients     map[string]*s3.Client
}

// newS3Driver builds the driver. It makes no request.
func newS3Driver(config S3Config, userAgent string) *s3Driver {
	return &s3Driver{
		config:       config,
		userAgent:    userAgent,
		lookupRegion: lookupBucketRegion,
		regions:      map[string]string{},
		clients:      map[string]*s3.Client{},
	}
}

// Stat implements Source with a HeadObject.
func (d *s3Driver) Stat(ctx context.Context, ref Ref) (Attributes, error) {
	var out *s3.HeadObjectOutput
	err := d.inBucketRegion(ctx, ref.Bucket, func(client *s3.Client) error {
		var callErr error
		out, callErr = client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket:    aws.String(ref.Bucket),
			Key:       aws.String(ref.Key),
			VersionId: optionalString(ref.VersionID),
		})
		return callErr
	})
	if err != nil {
		return Attributes{}, d.classify(ref, err)
	}
	if out.ContentLength == nil {
		return Attributes{}, fmt.Errorf("%w: %s", ErrNoSize, ref)
	}
	return Attributes{Size: *out.ContentLength, Marker: etagMarker(out.ETag)}, nil
}

// Open implements Source with a GetObject, pinned with If-Match and started at
// offset with Range.
func (d *s3Driver) Open(ctx context.Context, ref Ref, pin string, offset int64) (Opened, error) {
	input := &s3.GetObjectInput{
		Bucket:    aws.String(ref.Bucket),
		Key:       aws.String(ref.Key),
		VersionId: optionalString(ref.VersionID),
	}
	if pin != "" {
		etag, ok := strings.CutPrefix(pin, markerETag)
		if !ok {
			return Opened{}, fmt.Errorf("the marker %q is not one an S3 source records", pin)
		}
		input.IfMatch = aws.String(etag)
	}
	if offset > 0 {
		input.Range = aws.String("bytes=" + strconv.FormatInt(offset, 10) + "-")
	}

	var out *s3.GetObjectOutput
	err := d.inBucketRegion(ctx, ref.Bucket, func(client *s3.Client) error {
		var callErr error
		out, callErr = client.GetObject(ctx, input)
		return callErr
	})
	if err != nil {
		return Opened{}, d.classify(ref, err)
	}
	size := aws.ToInt64(out.ContentLength)
	if offset > 0 {
		size, err = rangedSize(ref, aws.ToString(out.ContentRange), offset)
	} else if out.ContentLength == nil {
		err = fmt.Errorf("%w: %s", ErrNoSize, ref)
	}
	if err != nil {
		_ = out.Body.Close()
		return Opened{}, err
	}
	return Opened{
		Body: out.Body,
		Size: size, Marker: etagMarker(out.ETag),
	}, nil
}

// optionalString returns nil for the empty string, the form the SDK reads as
// "not set".
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return aws.String(s)
}

// etagMarker turns an ETag into a marker. The value is kept exactly as the
// store sent it, quotes included, because it is sent back unchanged in
// If-Match.
func etagMarker(etag *string) string {
	if etag == nil || *etag == "" {
		return ""
	}
	return markerETag + *etag
}

// statusOf returns the HTTP status behind an SDK error, or 0.
func statusOf(err error) int {
	if response, ok := errors.AsType[*smithyhttp.ResponseError](err); ok {
		return response.HTTPStatusCode()
	}
	return 0
}

// headerOf returns one response header behind an SDK error, or "".
func headerOf(err error, name string) string {
	var response *smithyhttp.ResponseError
	if errors.As(err, &response) && response.Response != nil {
		return response.Response.Header.Get(name)
	}
	return ""
}

// classify turns an SDK error into one this package's callers can act on.
//
// A HEAD response has no body, so S3 cannot say why it refused: a 403 is the
// same for a missing permission, a wrong key and, for an identity without
// s3:ListBucket, an object that does not exist. The text says so in place of
// guessing.
//
// A credential that stops working after the client is built, such as an
// expired session token, fails inside the SDK before any request is sent, so
// it has no status. The SDK exports no type for it; its text is matched.
func (d *s3Driver) classify(ref Ref, err error) error {
	if errors.Is(err, ErrCredentials) {
		return err
	}
	switch statusOf(err) {
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, ref)
	case http.StatusForbidden:
		return fmt.Errorf("%w: %s. The identity needs s3:GetObject on the object. Without s3:ListBucket on the bucket, an object that does not exist is reported this way too: %w", ErrAccessDenied, ref, err)
	case http.StatusPreconditionFailed:
		return fmt.Errorf("%w: %s", ErrChanged, ref)
	}
	if strings.Contains(err.Error(), "get credentials") || strings.Contains(err.Error(), "retrieve credentials") {
		return fmt.Errorf("%w: reading %s: %w", ErrCredentials, ref, err)
	}
	return fmt.Errorf("reading %s: %w", ref, err)
}

// inBucketRegion runs call against the bucket's Region, finding that Region
// and running call once more when the first answer says the bucket is
// elsewhere.
func (d *s3Driver) inBucketRegion(ctx context.Context, bucket string, call func(*s3.Client) error) error {
	client, region, err := d.client(ctx, bucket)
	if err != nil {
		return err
	}
	err = call(client)
	if err == nil || d.customEndpoint() {
		return err
	}
	actual, moved := d.movedTo(ctx, bucket, region, err)
	if !moved {
		return err
	}
	tflog.Debug(ctx, "S3 bucket is in another Region; retrying there", map[string]any{
		"bucket": bucket, "tried": region, "region": actual,
	})
	d.mu.Lock()
	d.regions[bucket] = actual
	d.mu.Unlock()
	client, _, err = d.client(ctx, bucket)
	if err != nil {
		return err
	}
	return call(client)
}

// customEndpoint reports whether S3 requests go somewhere other than AWS:
// file_sources.s3.endpoints.s3 is set, or one of the environment variables the
// SDK reads an endpoint from is. Such a store has one Region and no AWS STS,
// so the Region chase and credential validation are both skipped for it. An
// endpoint_url in a shared configuration profile is not detected; a store
// configured that way needs endpoints.s3 set as well.
func (d *s3Driver) customEndpoint() bool {
	return d.config.EndpointS3 != "" || os.Getenv("AWS_ENDPOINT_URL_S3") != "" || os.Getenv("AWS_ENDPOINT_URL") != ""
}

// movedTo reports the Region a bucket is in when err is S3's answer to a
// request sent to the wrong one: a 301, or the 400 the global endpoint gives
// for a signature made for another Region.
func (d *s3Driver) movedTo(ctx context.Context, bucket, tried string, err error) (string, bool) {
	status := statusOf(err)
	if status != http.StatusMovedPermanently && status != http.StatusBadRequest {
		return "", false
	}
	region := headerOf(err, bucketRegionHeader)
	if region == "" && status == http.StatusMovedPermanently && inCommercialPartition(tried) {
		looked, lookupErr := d.lookupRegion(ctx, bucket)
		if lookupErr != nil {
			tflog.Debug(ctx, "could not find the S3 bucket's Region", map[string]any{"bucket": bucket, "err": lookupErr.Error()})
		}
		region = looked
	}
	return region, region != "" && region != tried
}

// inCommercialPartition reports whether a Region belongs to the standard AWS
// partition. The unauthenticated lookup asks the standard partition's global
// endpoint, where a China or GovCloud bucket's name may belong to somebody
// else's bucket, so it is only used from there.
func inCommercialPartition(region string) bool {
	return !strings.HasPrefix(region, "cn-") && !strings.HasPrefix(region, "us-gov-") && !strings.HasPrefix(region, "us-iso")
}

// lookupBucketRegion asks S3 which Region a bucket is in with an
// unauthenticated HEAD on the bucket. S3 answers 403 or 200, and either way
// names the Region in a header, so the lookup needs no credential and no
// permission (probed 2026-11-22). It does not work for a bucket whose name
// holds a dot, which cannot be addressed by hostname over TLS; such a bucket
// needs file_sources.s3.region set to its own Region.
func lookupBucketRegion(ctx context.Context, bucket string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, regionLookupTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, "https://"+bucket+".s3.amazonaws.com/", nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	_ = resp.Body.Close()
	return resp.Header.Get(bucketRegionHeader), nil
}

// client returns the S3 client for the bucket's Region, and that Region.
func (d *s3Driver) client(ctx context.Context, bucket string) (*s3.Client, string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.base == nil {
		if d.buildErr != nil && time.Since(d.buildFailed) < buildFailureHold {
			return nil, "", d.buildErr
		}
		base, err := d.build(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil, "", fmt.Errorf("building the S3 client: %w", ctx.Err())
			}
			d.buildErr, d.buildFailed = err, time.Now()
			return nil, "", err
		}
		d.base, d.buildErr = &base, nil
	}
	region := d.regions[bucket]
	if region == "" {
		region = d.base.Region
	}
	if client, ok := d.clients[region]; ok {
		return client, region, nil
	}
	client := s3.NewFromConfig(*d.base, func(o *s3.Options) {
		o.Region = region
		o.UsePathStyle = d.config.UsePathStyle
		if d.config.EndpointS3 != "" {
			o.BaseEndpoint = aws.String(d.config.EndpointS3)
		}
	})
	d.clients[region] = client
	return client, region, nil
}

// build resolves credentials and returns the AWS configuration every S3
// client is made from. The caller holds the lock, so concurrent first uses
// collapse into one build.
//
// The credential module is given no logger. With one, it writes each AWS STS
// exchange to the debug log, and its masking covers access key IDs and secret
// keys only: the OpenID Connect token sent to assume a role and the session
// token that comes back are both printed whole (found by running it against a
// stand-in STS). What the provider logs about a read it logs itself, in
// Resolver.
func (d *s3Driver) build(ctx context.Context) (aws.Config, error) {
	settings := &awsbase.Config{
		AccessKey:              d.config.AccessKey,
		SecretKey:              d.config.SecretKey,
		Token:                  d.config.Token,
		Profile:                d.config.Profile,
		Region:                 d.config.Region,
		SharedConfigFiles:      d.config.SharedConfigFiles,
		SharedCredentialsFiles: d.config.SharedCredentialsFiles,
		StsEndpoint:            d.config.EndpointSTS,
		SkipCredsValidation:    d.customEndpoint() && d.config.EndpointSTS == "",
		CallerName:             "the Jamf Platform provider's file_sources.s3 block",
		CallerDocumentationURL: GuideURL,
	}
	if d.userAgent != "" {
		name, version, _ := strings.Cut(d.userAgent, "/")
		settings.UserAgent = awsbase.UserAgentProducts{{Name: name, Version: version}}
	}
	if role := d.config.AssumeRole; role != nil {
		settings.AssumeRole = []awsbase.AssumeRole{{
			RoleARN:     role.RoleARN,
			SessionName: role.SessionName,
			ExternalID:  role.ExternalID,
		}}
	}
	if role := d.config.AssumeRoleWithWebIdentity; role != nil {
		settings.AssumeRoleWithWebIdentity = &awsbase.AssumeRoleWithWebIdentity{
			RoleARN:              role.RoleARN,
			SessionName:          role.SessionName,
			WebIdentityToken:     role.WebIdentityToken,
			WebIdentityTokenFile: role.WebIdentityTokenFile,
		}
	}

	_, base, diags := awsbase.GetAwsConfig(ctx, settings)
	if err := credentialError(ctx, diags); err != nil {
		return aws.Config{}, err
	}
	if base.Region == "" {
		base.Region = fallbackRegion
	}
	return base, nil
}

// credentialError turns the credential module's diagnostics into one error,
// logging its warnings. Nil when there is no error among them. The module
// lays its detail out over several lines for a terminal; the line breaks are
// folded, because the text ends up inside a diagnostic that does its own
// wrapping.
func credentialError(ctx context.Context, diags awsbasediag.Diagnostics) error {
	var problems []string
	for _, d := range diags {
		text := strings.Join(strings.Fields(d.Summary()+": "+d.Detail()), " ")
		if d.Severity() == awsbasediag.SeverityError {
			problems = append(problems, text)
			continue
		}
		tflog.Warn(ctx, "AWS credential warning", map[string]any{"warning": text})
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrCredentials, strings.Join(problems, "; "))
}
