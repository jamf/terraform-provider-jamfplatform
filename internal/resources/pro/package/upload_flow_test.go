// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

const flowContent = "package bytes used by the route tests"

func useRecorderForDirectUploads(t *testing.T, rec *routeRecorder) {
	t.Helper()
	original := directUpload
	directUpload = rec.directUpload
	t.Cleanup(func() { directUpload = original })
}

func convergedRecord(t *testing.T, content, fileName string) pro.Package {
	t.Helper()
	digests, size, err := hashStream(strings.NewReader(content))
	if err != nil {
		t.Fatalf("hashStream: %v", err)
	}
	rec := record(cloudStatusReady, strconv.FormatInt(size, 10), digests.sha3, "")
	rec.FileName = fileName
	return rec
}

func failedUploadRecord() pro.Package {
	return record(cloudStatusReady, "0", "dead", "")
}

func routeUploadEvent(route uploadRoute, id, fileName string) string {
	if route == uploadViaJCDS {
		return "upload-jcds:" + fileName
	}
	return "upload-api:" + id + ":" + fileName
}

func assertOnlyRoute(t *testing.T, rec *routeRecorder, route uploadRoute, wantUploads int, id, fileName string) {
	t.Helper()
	other := uploadViaAPI
	if route == uploadViaAPI {
		other = uploadViaJCDS
	}
	prefix := func(r uploadRoute) string {
		if r == uploadViaJCDS {
			return "upload-jcds:"
		}
		return "upload-api:"
	}
	if got := rec.count(prefix(other)); got != 0 {
		t.Errorf("%v route seen %d times while %v was chosen; events %v", other, got, route, rec.snapshot())
	}
	want := routeUploadEvent(route, id, fileName)
	got := 0
	for _, e := range rec.snapshot() {
		if e == want {
			got++
		}
	}
	if got != wantUploads {
		t.Errorf("event %q seen %d times, want %d; events %v", want, got, wantUploads, rec.snapshot())
	}
}

var bothRoutes = []uploadRoute{uploadViaJCDS, uploadViaAPI}

func TestOpenHashUploadVerifyReachesTheChosenRoute(t *testing.T) {
	for _, route := range bothRoutes {
		for _, retry := range []bool{false, true} {
			name := route.String() + " route"
			if retry {
				name += " retried after a failed upload"
			}
			t.Run(name, func(t *testing.T) {
				rec := &routeRecorder{}
				useRecorderForDirectUploads(t, rec)
				good := convergedRecord(t, flowContent, "app.pkg")
				rec.getFn = func(call int) pro.Package {
					if retry && call <= 2 {
						return failedUploadRecord()
					}
					return good
				}

				src := filepath.Join(t.TempDir(), "app.pkg")
				if err := os.WriteFile(src, []byte(flowContent), 0o600); err != nil {
					t.Fatal(err)
				}
				plan := &PackageResourceModel{
					ID:                types.StringValue("9"),
					FileName:          types.StringValue("app.pkg"),
					PackageFileSource: types.StringValue(src),
				}

				diags := openHashUploadVerify(context.Background(), rec, route, plan, fileDigests{}, "")
				if diags.HasError() {
					t.Fatalf("openHashUploadVerify: %v", diags)
				}
				wantUploads := 1
				if retry {
					wantUploads = 2
				}
				assertOnlyRoute(t, rec, route, wantUploads, "9", "app.pkg")
			})
		}
	}
}

func newContentServer(t *testing.T, declareLength bool, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		if declareLength {
			w.Header().Set("Content-Length", strconv.Itoa(len(flowContent)))
		}
		_, _ = w.Write([]byte(flowContent))
		if !declareLength {
			w.(http.Flusher).Flush()
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestStreamURLUploadAndVerifyReachesTheChosenRoute(t *testing.T) {
	cases := []struct {
		name          string
		declareLength bool
		retry         bool
		wantFetches   int32
		wantUploads   int
	}{
		{name: "streamed", declareLength: true, wantFetches: 1, wantUploads: 1},
		{name: "streamed and retried", declareLength: true, retry: true, wantFetches: 2, wantUploads: 2},
		{name: "no Content-Length falls back to disk staging", declareLength: false, wantFetches: 2, wantUploads: 1},
		{name: "no Content-Length falls back and retries", declareLength: false, retry: true, wantFetches: 2, wantUploads: 2},
	}
	for _, route := range bothRoutes {
		for _, tc := range cases {
			t.Run(route.String()+" route "+tc.name, func(t *testing.T) {
				rec := &routeRecorder{}
				useRecorderForDirectUploads(t, rec)
				good := convergedRecord(t, flowContent, "app.pkg")
				rec.getFn = func(call int) pro.Package {
					if tc.retry && call <= 2 {
						return failedUploadRecord()
					}
					return good
				}

				var hits atomic.Int32
				srv := newContentServer(t, tc.declareLength, &hits)
				unique := strings.NewReplacer(" ", "-", "/", "-").Replace(t.Name())
				plan := &PackageResourceModel{
					ID:                types.StringValue("9"),
					FileName:          types.StringValue("app.pkg"),
					PackageFileSource: types.StringValue(srv.URL + "/" + unique + ".pkg"),
				}

				diags := streamURLUploadAndVerify(context.Background(), rec, route, plan, fileDigests{}, "")
				if diags.HasError() {
					t.Fatalf("streamURLUploadAndVerify: %v", diags)
				}
				assertOnlyRoute(t, rec, route, tc.wantUploads, "9", "app.pkg")
				if hits.Load() != tc.wantFetches {
					t.Errorf("origin fetched %d times, want %d", hits.Load(), tc.wantFetches)
				}
			})
		}
	}
}

func renamePlan(fileName string) (plan, state *PackageResourceModel) {
	plan = &PackageResourceModel{ID: types.StringValue("9"), FileName: types.StringValue(fileName)}
	state = &PackageResourceModel{ID: types.StringValue("9"), FileName: types.StringValue("app-1.0.pkg"), Size: types.StringValue("10")}
	return plan, state
}

func TestReuploadRenamesTheRecordBeforeADirectUpload(t *testing.T) {
	cases := []struct {
		name       string
		route      uploadRoute
		newName    string
		wantRename bool
	}{
		{"jcds route, name changed", uploadViaJCDS, "app-1.1.pkg", true},
		{"jcds route, name unchanged", uploadViaJCDS, "app-1.0.pkg", false},
		{"api route, name changed", uploadViaAPI, "app-1.1.pkg", false},
	}
	for _, tc := range cases {
		run := func(t *testing.T, fromURL bool) {
			rec := &routeRecorder{}
			useRecorderForDirectUploads(t, rec)
			rec.getFn = func(int) pro.Package {
				p := convergedRecord(t, flowContent, "app-1.0.pkg")
				p.PackageName = "App"
				return p
			}
			plan, state := renamePlan(tc.newName)

			var diags diag.Diagnostics
			if fromURL {
				var hits atomic.Int32
				srv := newContentServer(t, true, &hits)
				plan.PackageFileSource = types.StringValue(srv.URL + "/rename-" + strings.ReplaceAll(t.Name(), "/", "-") + ".pkg")
				diags = reuploadFromURL(context.Background(), rec, tc.route, plan, state, fileDigests{})
			} else {
				local, size, err := hashStream(strings.NewReader(flowContent))
				if err != nil {
					t.Fatal(err)
				}
				diags = reuploadFromFile(context.Background(), rec, tc.route, plan, state, tc.newName, strings.NewReader(flowContent), local, size, fileDigests{})
			}
			if diags.HasError() {
				t.Fatalf("re-upload: %v", diags)
			}

			events := rec.snapshot()
			put := slices.Index(events, "put:"+tc.newName)
			upload := slices.Index(events, routeUploadEvent(tc.route, "9", tc.newName))
			if upload < 0 {
				t.Fatalf("no upload recorded; events %v", events)
			}
			if !tc.wantRename {
				if len(rec.puts) != 0 {
					t.Fatalf("unexpected record write %v; events %v", rec.puts, events)
				}
				return
			}
			if put < 0 || put > upload {
				t.Fatalf("rename PUT must precede the upload; events %v", events)
			}
			if len(rec.puts) != 1 || rec.puts[0].Size != nil || rec.puts[0].PackageName != "App" {
				t.Errorf("rename PUT should echo the server record without size; got %+v", rec.puts)
			}
		}
		t.Run(tc.name+" from a file", func(t *testing.T) { run(t, false) })
		t.Run(tc.name+" from a URL", func(t *testing.T) { run(t, true) })
	}
}

func TestRenameRecordBeforeUploadSkipsAnAlreadyRenamedRecord(t *testing.T) {
	rec := &routeRecorder{getFn: func(int) pro.Package { return pro.Package{FileName: "app-1.1.pkg"} }}
	plan, state := renamePlan("app-1.1.pkg")
	if diags := renameRecordBeforeUpload(context.Background(), rec, uploadViaJCDS, plan, state); diags.HasError() {
		t.Fatal(diags)
	}
	if len(rec.puts) != 0 {
		t.Fatalf("a record already carrying the name must not be written; got %v", rec.puts)
	}
}
