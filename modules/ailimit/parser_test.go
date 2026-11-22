package ailimit

import (
	"testing"
	"time"
)

func TestParseUsageOutput_TSV(t *testing.T) {
	input := `ERROR: logging before google.Init: I1122 12:22:30.552983       1 server_zerog.go:46] Initialized Android local timezone to Asia/Bangkok
Gemini Models	Weekly Limit Remaining	64%	2026-11-28T08:50:17Z
Gemini Models	Five Hour Limit Remaining	81%	2026-11-22T09:38:36Z
Claude and GPT models	Weekly Limit Remaining	3%	2026-11-26T16:48:07Z
Claude and GPT models	Five Hour Limit Remaining	100%	2026-11-22T10:23:00Z`

	now := time.Now()
	buckets, err := ParseUsageOutput(input, now)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if len(buckets) != 4 {
		t.Fatalf("expected 4 buckets, got %d", len(buckets))
	}

	b0 := buckets[0]
	if b0.BucketID != "gemini-weekly" || b0.RemainingPercent != 64.0 || b0.Window != "weekly" {
		t.Errorf("bucket 0 mismatch: %+v", b0)
	}

	b1 := buckets[1]
	if b1.BucketID != "gemini-5h" || b1.RemainingPercent != 81.0 || b1.Window != "5h" {
		t.Errorf("bucket 1 mismatch: %+v", b1)
	}

	b2 := buckets[2]
	if b2.BucketID != "3p-weekly" || b2.RemainingPercent != 3.0 || !b2.IsLow {
		t.Errorf("bucket 2 mismatch: %+v", b2)
	}

	b3 := buckets[3]
	if b3.BucketID != "3p-5h" || b3.RemainingPercent != 100.0 {
		t.Errorf("bucket 3 mismatch: %+v", b3)
	}
}

func TestParseUsageOutput_MacAlignedFormat(t *testing.T) {
	input := `Quota:
Gemini Models          Weekly Limit Remaining     61%   2026-11-28 15:50 +07
Gemini Models          Five Hour Limit Remaining  70%   2026-11-22 16:38 +07
Claude and GPT models  Weekly Limit Remaining     3%    2026-11-26 23:48 +07
Claude and GPT models  Five Hour Limit Remaining  100%  2026-11-22 17:45 +07`

	now := time.Now()
	buckets, err := ParseUsageOutput(input, now)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if len(buckets) != 4 {
		t.Fatalf("expected 4 buckets, got %d", len(buckets))
	}

	if buckets[0].RemainingPercent != 61.0 || buckets[0].BucketID != "gemini-weekly" {
		t.Errorf("expected 61%% for gemini-weekly, got %+v", buckets[0])
	}
	if buckets[1].RemainingPercent != 70.0 || buckets[1].BucketID != "gemini-5h" {
		t.Errorf("expected 70%% for gemini-5h, got %+v", buckets[1])
	}
	if buckets[2].RemainingPercent != 3.0 || !buckets[2].IsLow || buckets[2].BucketID != "3p-weekly" {
		t.Errorf("expected 3%% is_low for 3p-weekly, got %+v", buckets[2])
	}
	if buckets[3].RemainingPercent != 100.0 || buckets[3].BucketID != "3p-5h" {
		t.Errorf("expected 100%% for 3p-5h, got %+v", buckets[3])
	}
}
