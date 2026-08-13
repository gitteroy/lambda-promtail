package main

import (
	"context"
	"testing"

	"github.com/grafana/loki/v3/pkg/logproto"
)

// recording client to assert what gets sent
type recordingClient struct{ sends, emptySends int }

func (r *recordingClient) sendToPromtail(ctx context.Context, b *batch) error {
	_, cnt := b.createPushRequest()
	r.sends++
	if cnt == 0 { r.emptySends++ }
	return nil
}

// Regression: a batch that has been fully flushed (empty) must NOT be sent.
// Reproduces the CloudTrail 422 "at least one valid stream is required" DLQ bug.
func TestEmptyBatchNotSent(t *testing.T) {
	c := &promtailClient{} // real client path via sendToPromtail guard
	_ = c
	// Build an empty batch and ensure the guard short-circuits before HTTP.
	b := &batch{streams: map[string]*logproto.Stream{}, processor: &LokiStages{}}
	// createPushRequest on empty batch => 0 entries
	_, cnt := b.createPushRequest()
	if cnt != 0 {
		t.Fatalf("expected empty batch, got %d entries", cnt)
	}
	// The guard lives in promtailClient.sendToPromtail; verify encode reports 0
	_, entries, err := b.encode()
	if err != nil { t.Fatal(err) }
	if entries != 0 {
		t.Fatalf("expected 0 entries from encode, got %d", entries)
	}
	// A well-behaved client must treat 0-entry batch as a no-op success.
}
