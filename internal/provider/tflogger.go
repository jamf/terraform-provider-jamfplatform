// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/plisthelpers"
)

// Ensure TerraformLogger implements client.Logger interface
var _ jamfplatform.Logger = (*TerraformLogger)(nil)

// TerraformLogger implements the client.Logger interface using tflog
type TerraformLogger struct{}

// NewTerraformLogger creates a new TerraformLogger
func NewTerraformLogger() *TerraformLogger {
	return &TerraformLogger{}
}

// maxLoggedBodyBytes caps the rendered body length before truncation. Applied
// to the redacted, pretty-printed string so truncation never lands mid-token
// and never exposes content that redaction would otherwise have removed.
const maxLoggedBodyBytes = 5000

// redactedPlaceholder replaces every value the body redactor treats as
// credential material.
const redactedPlaceholder = "REDACTED"

// multipartSentinel is the literal stand-in the SDK passes to LogRequest in
// place of a multipart body, so file and package uploads are never rendered.
// Recognised by formatBody so it survives the fail-closed path rather than
// being reported as an unparseable body.
const multipartSentinel = "<multipart body>"

// LogRequest logs HTTP request details using tflog at DEBUG level. Bodies are
// redacted, then pretty-printed by content sniff (JSON / XML). The SDK's Logger
// interface passes no request headers, so there is nothing to redact there —
// the bearer token never reaches this function.
func (l *TerraformLogger) LogRequest(ctx context.Context, method, url string, body []byte) {
	fields := map[string]any{
		"method": method,
		"url":    url,
	}

	if len(body) > 0 {
		fields["request_body"] = formatBody(body)
	}

	tflog.Debug(ctx, "HTTP Request", fields)
}

// LogResponse logs HTTP response details using tflog at DEBUG level. Bodies are
// redacted, then pretty-printed by content sniff (JSON / XML).
func (l *TerraformLogger) LogResponse(ctx context.Context, statusCode int, headers http.Header, body []byte) {
	fields := map[string]any{
		"status_code": statusCode,
	}

	if len(headers) > 0 {
		fields["response_headers"] = formatHeaders(headers)
	}

	if len(body) > 0 {
		fields["response_body"] = formatBody(body)
	}

	tflog.Debug(ctx, "HTTP Response", fields)
}

// formatBody renders a request or response body for a debug log: credential
// values are replaced with redactedPlaceholder, the remainder is pretty-printed,
// and the result is truncated to keep logs readable. Redaction runs before
// truncation, so a secret early in a long body is removed rather than merely
// pushed past the cut.
//
// The renderer FAILS CLOSED. Redaction can only find secrets in a body it can
// parse, so a body that is neither JSON nor XML is withheld entirely rather than
// logged raw — logging it would silently bypass every rule below. The multipart
// sentinel is the one recognised exception.
func formatBody(body []byte) string {
	if string(bytes.TrimSpace(body)) == multipartSentinel {
		return multipartSentinel
	}

	rendered, ok := redactAndFormat(body)
	if !ok {
		return fmt.Sprintf("<body withheld: %d bytes, not parseable as JSON or XML so it cannot be redaction-checked>", len(body))
	}

	if len(rendered) > maxLoggedBodyBytes {
		return rendered[:maxLoggedBodyBytes] + "... (truncated)"
	}
	return rendered
}

// redactAndFormat sniffs the first non-whitespace byte to pick a renderer.
// '{' or '[' selects JSON, '<' selects XML. Anything else is unsupported and
// reports false so the caller withholds the body.
func redactAndFormat(body []byte) (string, bool) {
	leading := bytes.TrimLeft(body, " \t\r\n")
	if len(leading) == 0 {
		return string(body), true
	}

	switch leading[0] {
	case '{', '[':
		return redactJSON(body)
	case '<':
		return redactXML(body)
	}
	return "", false
}

// sensitiveHeaders are response headers whose values may carry credentials or
// session state. Logged as redactedPlaceholder to avoid leaks in tflog output.
// Compared case-insensitively against canonical MIME keys (http.Header
// normalises to canonical form).
var sensitiveHeaders = map[string]struct{}{
	"Authorization":       {},
	"Proxy-Authorization": {},
	"Cookie":              {},
	"Set-Cookie":          {},
	"X-Api-Key":           {},
}

// sensitiveBodyFields are JSON and XML wire field names whose values are
// credential material: plaintext passwords and their _sha256 echoes, recovery
// and encryption keys, client secrets and credential bundles, bearer and
// enrolment tokens, keystore blobs, and licence keys.
//
// Keys are lower-cased and matching is EXACT after lower-casing, never by
// substring. Exact matching is deliberate. The Jamf Platform and Jamf Pro
// schemas carry many non-secret fields whose names contain "password", "key" or
// "token" — passwordMinLength, passwordMaxAge, passwordHistoryDepth,
// passcodePresent, passed, passPercentage, keyUsage, groupRdnKey, triggerKey,
// tokenExpiration, bootstrapTokenEscrowed, vppTokenEnabled among them. Those are
// frequently the very values an operator enabled DEBUG to inspect, so a
// substring rule would redact the log into uselessness while adding no safety.
//
// Bare "key" and "keys" are deliberately absent: they hold key material in
// SsoKeystore but are plain identifiers in StatusItem, UserPreferencesSettings
// and ComputerContentCachingDataMigrationErrorUserInfo. The blob-bearing
// siblings below cover the material without hiding those identifiers.
//
// Two entries come from a tool vendor's settings body rather than a Jamf schema:
// env and apikeyhelper, the AI Governance policy settings where a deployment
// pins a model-provider API key or names a command that prints one. env holds a
// map, and a matched member's whole value is consumed, so the entire map is
// withheld rather than one leaf of it.
//
// Both camelCase (Jamf Pro JSON) and snake_case (ProClassic XML) spellings are
// listed because they are distinct strings; case variants are not, since
// matching lower-cases first.
var sensitiveBodyFields = map[string]struct{}{
	"accesskeyid":                {},
	"access_token":               {},
	"adminpassword":              {},
	"airplaypassword":            {},
	"airplay_password":           {},
	"apikeyhelper":               {},
	"applecaretoken":             {},
	"basicauthcredentials":       {},
	"bootstraptoken":             {},
	"cached_credentials":         {},
	"clientsecret":               {},
	"currentpassword":            {},
	"encodedtoken":               {},
	"encryption_key":             {},
	"env":                        {},
	"googlemailcredentials":      {},
	"graphapicredentials":        {},
	"gsxkeystore":                {},
	"httpspassword":              {},
	"http_password":              {},
	"http_password_sha256":       {},
	"identitykeystore":           {},
	"institutional_recovery_key": {},
	"keystore":                   {},
	"keystorebytes":              {},
	"keystorefile":               {},
	"keystorepassword":           {},
	"lapsuserpasswordlist":       {},
	"managed_password":           {},
	"newpassword":                {},
	"of_password":                {},
	"of_password_sha256":         {},
	"open_firmware_efi_password": {},
	"password":                   {},
	"password_sha256":            {},
	"personalrecoverykey":        {},
	"pin":                        {},
	"plainpassword":              {},
	"privatekey":                 {},
	"productkey":                 {},
	"readonlypassword":           {},
	"readwritepassword":          {},
	"read_only_password":         {},
	"read_only_password_sha256":  {},
	"read_write_password":        {},
	"read_write_password_sha256": {},
	"recoverylockpassword":       {},
	"refreshtoken":               {},
	"secretaccesskey":            {},
	"servertoken":                {},
	"servicetoken":               {},
	"service_token":              {},
	"sessiontoken":               {},
	"sshpassword":                {},
	"ssh_password":               {},
	"ssh_password_sha256":        {},
	"token":                      {},
	"unlocktoken":                {},
}

// sensitivePlistKeys are Apple configuration-profile payload keys whose values
// are credential material. Lower-cased and matched exactly, for the same reason
// as sensitiveBodyFields. PayloadContent is absent because it needs its value to
// decide — see isSensitivePlistKey.
var sensitivePlistKeys = map[string]struct{}{
	"authpassword":     {},
	"clientsecret":     {},
	"otpsecret":        {},
	"outgoingpassword": {},
	"passphrase":       {},
	"password":         {},
	"privatekey":       {},
	"proxypassword":    {},
	"psk":              {},
	"routingpassword":  {},
	"secret":           {},
	"sharedsecret":     {},
	"token":            {},
}

// isSensitiveField reports whether a JSON or XML wire field name carries
// credential material.
func isSensitiveField(name string) bool {
	_, ok := sensitiveBodyFields[strings.ToLower(name)]
	return ok
}

// isSensitivePlistKey reports whether a plist key carries credential material.
//
// PayloadContent needs its value to decide. At the top level of a .mobileconfig
// it is the array of payload dictionaries, and redacting it would blank the
// entire profile; inside a certificate payload the same key holds the raw
// PKCS#12 or DER bytes, which must be redacted. Container values are therefore
// walked and scalar values are redacted.
func isSensitivePlistKey(key string, value any) bool {
	lower := strings.ToLower(key)

	if lower == "payloadcontent" {
		switch value.(type) {
		case []any, map[string]any:
			return false
		}
		return true
	}

	_, ok := sensitivePlistKeys[lower]
	return ok
}

// formatHeaders renders an http.Header as a sorted multi-line block of
// "Key: value" pairs. Multi-value headers are comma-joined. Sensitive headers
// (Authorization, Set-Cookie, etc.) are redacted.
func formatHeaders(h http.Header) string {
	if len(h) == 0 {
		return ""
	}
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(k)
		sb.WriteString(": ")
		if _, redact := sensitiveHeaders[http.CanonicalHeaderKey(k)]; redact {
			sb.WriteString(redactedPlaceholder)
			continue
		}
		sb.WriteString(strings.Join(h[k], ", "))
	}
	return sb.String()
}

// redactJSON re-emits a JSON body with sensitive values replaced and two-space
// indentation applied. Reports false if the body is not a single well-formed
// JSON value, so the caller withholds it.
//
// It streams tokens rather than unmarshalling into map[string]any and
// re-marshalling, because Go maps do not preserve object member order. Wire
// field order has been load-bearing in this provider before — Jamf Pro rejects
// some classic payloads whose members arrive out of order — so a debug log that
// silently re-sorted members would misdirect that exact investigation.
// UseNumber preserves numeric literals, which the default float64 decode would
// rewrite for large integers and trailing zeros.
func redactJSON(body []byte) (string, bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	var buf bytes.Buffer
	if err := writeJSONValue(dec, &buf, 0); err != nil {
		return "", false
	}

	if _, err := dec.Token(); err != io.EOF {
		return "", false
	}

	return buf.String(), true
}

// writeJSONValue reads exactly one JSON value from dec and writes its redacted,
// indented rendering to buf.
func writeJSONValue(dec *json.Decoder, buf *bytes.Buffer, depth int) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}

	delim, isDelim := tok.(json.Delim)
	if !isDelim {
		writeJSONScalar(buf, tok)
		return nil
	}

	switch delim {
	case '{':
		return writeJSONObject(dec, buf, depth)
	case '[':
		return writeJSONArray(dec, buf, depth)
	}
	return fmt.Errorf("unexpected JSON delimiter %v", delim)
}

// writeJSONObject renders an object whose opening brace has already been read.
// A member whose name is sensitive has its entire value consumed and replaced,
// so a credential nested under a sensitive key cannot survive.
func writeJSONObject(dec *json.Decoder, buf *bytes.Buffer, depth int) error {
	buf.WriteByte('{')
	empty := true

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("non-string JSON object key %v", keyTok)
		}

		if !empty {
			buf.WriteByte(',')
		}
		empty = false
		writeJSONIndent(buf, depth+1)
		buf.WriteString(strconv.Quote(key))
		buf.WriteString(": ")

		if isSensitiveField(key) {
			var discard json.RawMessage
			if err := dec.Decode(&discard); err != nil {
				return err
			}
			buf.WriteString(strconv.Quote(redactedPlaceholder))
			continue
		}

		if err := writeJSONValue(dec, buf, depth+1); err != nil {
			return err
		}
	}

	if _, err := dec.Token(); err != nil {
		return err
	}
	if !empty {
		writeJSONIndent(buf, depth)
	}
	buf.WriteByte('}')
	return nil
}

// writeJSONArray renders an array whose opening bracket has already been read.
func writeJSONArray(dec *json.Decoder, buf *bytes.Buffer, depth int) error {
	buf.WriteByte('[')
	empty := true

	for dec.More() {
		if !empty {
			buf.WriteByte(',')
		}
		empty = false
		writeJSONIndent(buf, depth+1)
		if err := writeJSONValue(dec, buf, depth+1); err != nil {
			return err
		}
	}

	if _, err := dec.Token(); err != nil {
		return err
	}
	if !empty {
		writeJSONIndent(buf, depth)
	}
	buf.WriteByte(']')
	return nil
}

// writeJSONIndent writes a newline plus two spaces per depth level.
func writeJSONIndent(buf *bytes.Buffer, depth int) {
	buf.WriteByte('\n')
	buf.WriteString(strings.Repeat("  ", depth))
}

// writeJSONScalar writes a non-container JSON token.
func writeJSONScalar(buf *bytes.Buffer, tok json.Token) {
	switch v := tok.(type) {
	case nil:
		buf.WriteString("null")
	case string:
		buf.WriteString(strconv.Quote(v))
	case bool:
		buf.WriteString(strconv.FormatBool(v))
	case json.Number:
		buf.WriteString(v.String())
	default:
		buf.WriteString(strconv.Quote(fmt.Sprint(v)))
	}
}

// redactXML re-encodes an XML payload with two-space indentation, replacing the
// character data of any element whose name is a sensitive field. Reports false
// if the body is not well-formed XML, so the caller withholds it.
//
// Whitespace-only CharData tokens are dropped so existing wire whitespace does
// not double up against the new indentation.
//
// Character data that is itself a plist — how Jamf Pro carries configuration
// profile payloads inside <payloads> — is handed to redactPlist, because plist
// stores values as sibling <key>/<string> pairs that element-name matching
// cannot see.
func redactXML(body []byte) (string, bool) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")

	var stack []string

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", false
		}

		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if strings.TrimSpace(string(t)) == "" {
				continue
			}
			var parent string
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			switch {
			case isSensitiveField(parent):
				tok = xml.CharData(redactedPlaceholder)
			case looksLikePlist(t):
				tok = xml.CharData(redactPlist(t))
			}
		}

		if err := enc.EncodeToken(tok); err != nil {
			return "", false
		}
	}

	if err := enc.Flush(); err != nil {
		return "", false
	}
	return buf.String(), true
}

// looksLikePlist reports whether character data is an Apple property list, by
// sniffing for a plist or bare dict root with or without an XML prolog.
func looksLikePlist(data []byte) bool {
	trimmed := strings.TrimSpace(string(data))
	if !strings.HasPrefix(trimmed, "<") {
		return false
	}
	return strings.Contains(trimmed, "<plist") ||
		strings.Contains(trimmed, "<!DOCTYPE plist") ||
		strings.HasPrefix(trimmed, "<dict")
}

// redactPlist parses embedded plist content, replaces credential values, and
// re-emits it. A plist that will not parse is replaced wholesale, keeping the
// fail-closed contract of formatBody: unparsed content is never logged.
func redactPlist(raw []byte) string {
	parsed, _, err := plisthelpers.ParsePlist(raw)
	if err != nil {
		return redactedPlaceholder
	}

	redactPlistValue(parsed)

	out, err := plisthelpers.MarshalPlist(parsed)
	if err != nil {
		return redactedPlaceholder
	}
	return string(out)
}

// redactPlistValue walks a decoded plist in place, redacting sensitive keys, and
// returns the value it was given.
func redactPlistValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for k, v := range typed {
			if isSensitivePlistKey(k, v) {
				typed[k] = redactedPlaceholder
				continue
			}
			typed[k] = redactPlistValue(v)
		}
		return typed
	case []any:
		for i, v := range typed {
			typed[i] = redactPlistValue(v)
		}
		return typed
	}
	return value
}
