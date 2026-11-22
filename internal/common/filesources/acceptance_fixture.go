// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package filesources

import (
	"bytes"
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// PutForAcceptance writes body to an s3:// source and returns the version ID
// the store assigned, which is empty on a bucket without versioning.
//
// It exists for the acceptance suite, which has to create and replace the
// objects it reads, and is compiled only under the acceptance build tag: the
// provider itself never writes to a store, and the identity a user gives it
// needs read access alone. It goes through the driver's own client, so a test
// object is written with the credentials, endpoint and Region discovery the
// read under test will use.
func (r *Resolver) PutForAcceptance(ctx context.Context, src string, body []byte) (string, error) {
	ref, driver, err := r.s3ForAcceptance(src)
	if err != nil {
		return "", err
	}
	var out *s3.PutObjectOutput
	err = driver.inBucketRegion(ctx, ref.Bucket, func(client *s3.Client) error {
		var callErr error
		out, callErr = client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(ref.Bucket),
			Key:    aws.String(ref.Key),
			Body:   bytes.NewReader(body),
		})
		return callErr
	})
	if err != nil {
		return "", fmt.Errorf("writing %s: %w", ref, err)
	}
	return aws.ToString(out.VersionId), nil
}

// DeleteForAcceptance removes the object behind an s3:// source. On a
// versioned bucket this adds a delete marker and leaves the versions, which a
// lifecycle rule on the fixture bucket is expected to expire.
func (r *Resolver) DeleteForAcceptance(ctx context.Context, src string) error {
	ref, driver, err := r.s3ForAcceptance(src)
	if err != nil {
		return err
	}
	err = driver.inBucketRegion(ctx, ref.Bucket, func(client *s3.Client) error {
		_, callErr := client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(ref.Bucket),
			Key:    aws.String(ref.Key),
		})
		return callErr
	})
	if err != nil {
		return fmt.Errorf("deleting %s: %w", ref, err)
	}
	return nil
}

// s3ForAcceptance returns the S3 driver and the parsed source.
func (r *Resolver) s3ForAcceptance(src string) (Ref, *s3Driver, error) {
	ref, source, err := r.driver(src)
	if err != nil {
		return ref, nil, err
	}
	driver, ok := source.(*s3Driver)
	if !ok {
		return ref, nil, fmt.Errorf("%s is not an S3 source", ref)
	}
	return ref, driver, nil
}
