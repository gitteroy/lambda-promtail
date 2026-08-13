package main

import (
	"testing"

	"github.com/grafana/loki/v3/pkg/logproto"
)

// Regression: an empty batch must report zero entries so that
// promtailClient.sendToPromtail can short-circuit before issuing an HTTP push.
//
// A large multi-record S3 source (e.g. CloudTrail) whose total size is a clean
// multiple of batchSize flushes fully mid-stream, leaving the final batch empty.
// Sending an empty PushRequest makes Loki return
// "422: at least one valid stream is required for ingestion", which is
// non-retryable and sends the whole SQS message to the DLQ.
func TestEmptyBatchReportsZeroEntries(t *testing.T) {
	b := &batch{streams: map[string]*logproto.Stream{}, processor: &LokiStages{}}

	_, cnt := b.createPushRequest()
	if cnt != 0 {
		t.Fatalf("expected empty batch, got %d entries", cnt)
	}

	_, entries, err := b.encode()
	if err != nil {
		t.Fatal(err)
	}
	if entries != 0 {
		t.Fatalf("expected 0 entries from encode, got %d", entries)
	}
}
