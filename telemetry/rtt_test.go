package telemetry

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The gating logic is what protects Neon's autosuspend — worth pinning.

func TestActivityTrackerNeverTouchedIsInactive(t *testing.T) {
	a := NewActivityTracker()
	if a.activeWithin(time.Hour) {
		t.Fatal("fresh tracker reports active; probe would ping an idle DB")
	}
}

func TestActivityTrackerTouchActivates(t *testing.T) {
	a := NewActivityTracker()
	a.Touch()
	if !a.activeWithin(time.Minute) {
		t.Fatal("tracker inactive immediately after Touch")
	}
	if a.activeWithin(-time.Nanosecond) {
		t.Fatal("tracker active within a negative window")
	}
}

func TestActivityTrackerTrackMiddlewareTouches(t *testing.T) {
	a := NewActivityTracker()
	h := a.Track(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/orders", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("middleware altered response: got %d", rec.Code)
	}
	if !a.activeWithin(time.Minute) {
		t.Fatal("serving a request did not register as activity")
	}
}
