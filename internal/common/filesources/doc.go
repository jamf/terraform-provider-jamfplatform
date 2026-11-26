// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package filesources reads an upload source that lives in object storage:
// Amazon S3 (s3://bucket/key) and Azure Blob Storage
// (azblob://account/container/blob). It is the seam between the provider's
// file_sources configuration and a resource that uploads a file, and today it
// has one consumer, package_file_source on jamfplatform_pro_package.
//
// The split with internal/common/files is by where the bytes are. files owns
// a local path and a plain http(s) URL and is unchanged by this package.
// filesources owns the schemes that need a credential and expose a change
// marker, and it never opens a local path.
//
// # Shape
//
// The provider builds one Resolver per configured provider instance from the
// file_sources block and carries it on providerdata.Data. A resource hands the
// Resolver a source string and gets back a size, a marker and a reader. The
// API carries no resource types, so a second consumer needs nothing from the
// first.
//
// A Source is one store's driver. The Resolver picks the driver from the
// source's scheme, enforces the size cap before a byte is read, and fills in
// the file name. A scheme with no driver registered answers ErrNoDriver.
//
// # Source syntax
//
// Parse is the one place a source string is classified, so the plan-time
// validators and the Resolver cannot disagree about what a string means.
//
// An S3 key and an Azure blob name are taken literally, as the AWS CLI takes
// them: nothing is percent-decoded, and "#" and "%" are ordinary characters.
// The one query an S3 source accepts is a trailing ?versionId=, which reads
// that exact version. Any other "?" is refused, because a source that half
// looks like a URL would otherwise read a different object from the one its
// author meant. An Azure source takes no query at all.
//
// A scheme is matched without regard to case when a source is classified, so
// "S3://" is recognised for what it is and refused with a message that says
// to write it in lower case, in place of falling through to the local-path
// branch and failing as a missing file.
//
// # Unknown provider configuration
//
// A file_sources value derived from another resource is unknown while
// Terraform plans. The Resolver is still built, from the values that are
// known, and records the paths that were not in Unresolved. What a consumer
// does with that is the consumer's decision; this package only reports it.
package filesources
