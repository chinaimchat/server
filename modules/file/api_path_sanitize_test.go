package file

import "testing"

func TestSanitizeUploadPath(t *testing.T) {
	got := sanitizeUploadPath("/1776142170892/2026-04-14 11.54.18.jpg")
	want := "/1776142170892/2026-04-14_11.54.18.jpg"
	if got != want {
		t.Fatalf("sanitize upload path mismatch: got %q want %q", got, want)
	}

	got = sanitizeUploadPath(`\tmp\抖商 AI 学院?.png`)
	want = "/tmp/抖商_AI_学院_.png"
	if got != want {
		t.Fatalf("sanitize upload path with unicode mismatch: got %q want %q", got, want)
	}
}
