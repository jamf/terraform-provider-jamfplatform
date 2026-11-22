// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package filesources

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode"
)

// Kind is what a source string names.
type Kind int

// The kinds Parse distinguishes. KindLocal is the zero value because a string
// with no scheme is a filesystem path.
const (
	KindLocal Kind = iota
	KindHTTP
	KindHTTPS
	KindS3
	KindAzureBlob
	KindUnsupported
)

// The schemes Parse recognises, in the lower-case spelling a source must use.
const (
	SchemeHTTP      = "http"
	SchemeHTTPS     = "https"
	SchemeS3        = "s3"
	SchemeAzureBlob = "azblob"
)

// schemePattern matches a leading scheme followed by "://". The scheme is at
// least two characters so a Windows drive letter written with forward slashes
// ("C://packages/app.pkg") stays a local path.
var schemePattern = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]+)://`)

// bucketPattern is deliberately looser than the current S3 naming rules:
// buckets created before 2018 may hold upper-case letters and underscores, and
// refusing a name S3 serves would be worse than passing one it does not.
var bucketPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// versionPattern is an S3 version ID. Real ones are URL-safe, so anything else
// after ?versionId= is a second query parameter, a fragment or a typing slip.
var versionPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)

// namesAFile reports whether an object name can be an upload's file name: not
// empty, not a folder, and not ending in "." or "..", which path.Base would
// hand back as the file name.
func namesAFile(name string) bool {
	if name == "" || strings.HasSuffix(name, "/") {
		return false
	}
	last := path.Base(name)
	return last != "." && last != ".."
}

// accountPattern is the Azure storage account naming rule.
var accountPattern = regexp.MustCompile(`^[a-z0-9]{3,24}$`)

// Ref is a parsed source. Which fields are set depends on Kind: Bucket, Key
// and VersionID for KindS3; Account, Container and Blob for KindAzureBlob;
// none for the rest, which internal/common/files reads from the original
// string.
type Ref struct {
	Kind   Kind
	Scheme string

	Bucket    string
	Key       string
	VersionID string

	Account   string
	Container string
	Blob      string
}

// Native reports whether the source lives in a store this package reads.
func (r Ref) Native() bool {
	return r.Kind == KindS3 || r.Kind == KindAzureBlob
}

// LowerCaseScheme reports whether the scheme was written in lower case, the
// only spelling a source may use. A source with no scheme reports true.
func (r Ref) LowerCaseScheme() bool {
	return r.Scheme == strings.ToLower(r.Scheme)
}

// Filename is the last element of the object's name, the file name an upload
// falls back to when the resource does not set one. Empty for a source that
// is not native.
func (r Ref) Filename() string {
	switch r.Kind {
	case KindS3:
		return path.Base(r.Key)
	case KindAzureBlob:
		return path.Base(r.Blob)
	}
	return ""
}

// String renders a native source for a log line or a diagnostic. It is rebuilt
// from the parsed parts, so it can carry nothing the parser did not accept.
// Empty for a source that is not native: those strings can hold a signed
// query, and files.RedactSource is what renders them.
func (r Ref) String() string {
	switch r.Kind {
	case KindS3:
		s := SchemeS3 + "://" + r.Bucket + "/" + r.Key
		if r.VersionID != "" {
			s += "?versionId=" + r.VersionID
		}
		return s
	case KindAzureBlob:
		return SchemeAzureBlob + "://" + r.Account + "/" + r.Container + "/" + r.Blob
	}
	return ""
}

// SyntaxError is a source or a credential host that is not written the way
// this package reads it. Its text is one or more complete sentences addressed
// to whoever wrote the value, so a validator can use it as a diagnostic's
// detail unchanged.
type SyntaxError struct {
	detail string
}

// Error returns the explanation.
func (e *SyntaxError) Error() string { return e.detail }

// syntaxErrorf builds a SyntaxError.
func syntaxErrorf(format string, args ...any) error {
	return &SyntaxError{detail: fmt.Sprintf(format, args...)}
}

// Parse classifies a source string and, for a native one, splits it into its
// parts. The error is non-nil only for a native source that is malformed, and
// is then a *SyntaxError. An unrecognised scheme is
// not an error here: it comes back as KindUnsupported, and whether that is
// acceptable is the caller's rule.
func Parse(src string) (Ref, error) {
	m := schemePattern.FindStringSubmatch(src)
	if m == nil {
		return Ref{Kind: KindLocal}, nil
	}
	ref := Ref{Scheme: m[1]}
	rest := src[len(m[0]):]
	switch strings.ToLower(m[1]) {
	case SchemeS3, SchemeAzureBlob:
		if strings.ContainsFunc(src, unicode.IsControl) {
			ref.Kind = KindS3
			if strings.EqualFold(m[1], SchemeAzureBlob) {
				ref.Kind = KindAzureBlob
			}
			return ref, syntaxErrorf("The source holds a control character, such as a line break. Remove it.")
		}
	}
	switch strings.ToLower(m[1]) {
	case SchemeHTTP:
		ref.Kind = KindHTTP
	case SchemeHTTPS:
		ref.Kind = KindHTTPS
	case SchemeS3:
		ref.Kind = KindS3
		return parseS3(ref, rest)
	case SchemeAzureBlob:
		ref.Kind = KindAzureBlob
		return parseAzureBlob(ref, rest)
	default:
		ref.Kind = KindUnsupported
	}
	return ref, nil
}

// parseS3 splits "bucket/key[?versionId=id]".
func parseS3(ref Ref, rest string) (Ref, error) {
	const form = "An S3 source is written s3://bucket/key, with an optional ?versionId= suffix."
	bucket, key, found := strings.Cut(rest, "/")
	if !found || bucket == "" {
		return ref, syntaxErrorf("%s The bucket or the key is missing.", form)
	}
	if !bucketPattern.MatchString(bucket) {
		return ref, syntaxErrorf("%s The bucket name holds a character a bucket name cannot.", form)
	}
	if name, query, hasQuery := strings.Cut(key, "?"); hasQuery {
		version, isVersion := strings.CutPrefix(query, "versionId=")
		if !isVersion || !versionPattern.MatchString(version) {
			return ref, syntaxErrorf("%s No other query is read, and a key holding a literal \"?\" cannot be named.", form)
		}
		key = name
		ref.VersionID = version
	}
	if !namesAFile(key) {
		return ref, syntaxErrorf("%s The key is missing or names a folder.", form)
	}
	ref.Bucket = bucket
	ref.Key = key
	return ref, nil
}

// parseAzureBlob splits "account/container/blob".
func parseAzureBlob(ref Ref, rest string) (Ref, error) {
	const form = "An Azure Blob Storage source is written azblob://account/container/blob."
	account, remainder, found := strings.Cut(rest, "/")
	if !found || account == "" {
		return ref, syntaxErrorf("%s The storage account or the container is missing.", form)
	}
	if !accountPattern.MatchString(account) {
		return ref, syntaxErrorf("%s The storage account is 3 to 24 lower-case letters and digits: give the account name alone, without .blob.core.windows.net.", form)
	}
	container, blob, found := strings.Cut(remainder, "/")
	if !found || container == "" {
		return ref, syntaxErrorf("%s The container or the blob is missing.", form)
	}
	if strings.Contains(blob, "?") {
		return ref, syntaxErrorf("%s No query is read, so a snapshot, a version or a SAS token cannot be named here. A SAS URL is an https:// source.", form)
	}
	if !namesAFile(blob) {
		return ref, syntaxErrorf("%s The blob is missing or names a folder.", form)
	}
	ref.Account = account
	ref.Container = container
	ref.Blob = blob
	return ref, nil
}
