package web

import (
	"testing"
	"time"
)

func TestGRPCProcessSourceUsesLongerActionTimeout(t *testing.T) {
	source := NewGRPCProcessSource(12345)

	if source.readTimeout() != 2*time.Second {
		t.Fatalf("unexpected read timeout %s", source.readTimeout())
	}
	if source.actionTimeout() <= source.readTimeout() {
		t.Fatalf("expected action timeout to outlive read timeout, got read=%s action=%s", source.readTimeout(), source.actionTimeout())
	}
}
