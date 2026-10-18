// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmark

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const deletionTestBenchmarkID = "bench-485"

// Answers the deletion stub gives to a GET of the benchmark.
const (
	gone          = "404"
	flagged       = "deleted=true"
	notYetFlagged = "deleted=false"
)

// deletionStub answers GET /benchmarks/{id} from a fixed sequence, one entry
// per request, repeating the last entry once the sequence is exhausted. It
// counts the deletes it is sent and answers each with deleteStatus.
type deletionStub struct {
	mu           sync.Mutex
	answers      []string
	gets         int
	deletes      int
	deleteStatus int
}

func (s *deletionStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !strings.HasSuffix(r.URL.Path, "/benchmarks/"+deletionTestBenchmarkID) {
		w.WriteHeader(http.StatusTeapot)
		return
	}
	switch r.Method {
	case http.MethodDelete:
		s.deletes++
		w.WriteHeader(s.deleteStatus)
	case http.MethodGet:
		answer := s.answers[min(s.gets, len(s.answers)-1)]
		s.gets++
		if answer == gone {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		testhelpers.RespondJSON(w, http.StatusOK, compliancebenchmarks.BenchmarkResponseV2{
			BenchmarkID: deletionTestBenchmarkID,
			Deleted:     answer == flagged,
		})
	}
}

func (s *deletionStub) counts() (gets, deletes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gets, s.deletes
}

func runDeletionWait(t *testing.T, stub *deletionStub, timeout, redeleteAfter time.Duration) error {
	t.Helper()
	c := compliancebenchmarks.New(testhelpers.NewMockClient(t, stub))
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return waitForBenchmarkDeletion(ctx, c, deletionTestBenchmarkID, time.Millisecond, redeleteAfter)
}

// TestWaitForBenchmarkDeletion_WaitsFor404 pins issue #485: only a 404 ends the
// wait, whatever the deleted flag says before it.
func TestWaitForBenchmarkDeletion_WaitsFor404(t *testing.T) {
	cases := map[string][]string{
		"flag set then gone":      {notYetFlagged, flagged, flagged, gone},
		"flag never set":          {notYetFlagged, notYetFlagged, gone},
		"gone on the first poll":  {gone},
		"flagged for a long time": {flagged, flagged, flagged, flagged, flagged, gone},
	}
	for name, answers := range cases {
		t.Run(name, func(t *testing.T) {
			stub := &deletionStub{answers: answers, deleteStatus: http.StatusNoContent}
			if err := runDeletionWait(t, stub, 5*time.Second, time.Hour); err != nil {
				t.Fatalf("waitForBenchmarkDeletion: %v", err)
			}
			if gets, deletes := stub.counts(); gets != len(answers) || deletes != 0 {
				t.Errorf("gets=%d deletes=%d, want gets=%d deletes=0", gets, deletes, len(answers))
			}
		})
	}
}

// TestWaitForBenchmarkDeletion_ReissuesDelete checks the delete is sent again
// on every poll that finds the benchmark once redeleteAfter has passed.
func TestWaitForBenchmarkDeletion_ReissuesDelete(t *testing.T) {
	stub := &deletionStub{answers: []string{flagged, flagged, gone}, deleteStatus: http.StatusNoContent}
	if err := runDeletionWait(t, stub, 5*time.Second, 0); err != nil {
		t.Fatalf("waitForBenchmarkDeletion: %v", err)
	}
	if _, deletes := stub.counts(); deletes != 2 {
		t.Errorf("re-issued the delete %d times, want 2", deletes)
	}
}

// TestWaitForBenchmarkDeletion_ReissuedDelete404IsSuccess checks a re-issued
// delete answering 404 ends the wait even while GET still finds the benchmark.
func TestWaitForBenchmarkDeletion_ReissuedDelete404IsSuccess(t *testing.T) {
	stub := &deletionStub{answers: []string{flagged}, deleteStatus: http.StatusNotFound}
	if err := runDeletionWait(t, stub, 5*time.Second, 0); err != nil {
		t.Fatalf("waitForBenchmarkDeletion: %v", err)
	}
	if _, deletes := stub.counts(); deletes != 1 {
		t.Errorf("re-issued the delete %d times, want 1", deletes)
	}
}

// TestWaitForBenchmarkDeletion_TimeoutNamesLastFlag checks a benchmark that
// never goes away fails at the timeout with its last deleted flag in the error.
func TestWaitForBenchmarkDeletion_TimeoutNamesLastFlag(t *testing.T) {
	stub := &deletionStub{answers: []string{notYetFlagged, flagged}, deleteStatus: http.StatusNoContent}
	err := runDeletionWait(t, stub, time.Second, 0)
	if err == nil {
		t.Fatal("waitForBenchmarkDeletion succeeded for a benchmark that was never removed")
	}
	if !strings.Contains(err.Error(), "(deleted=true)") {
		t.Errorf("error %q does not name the last deleted flag", err)
	}
}
