package web

import (
	"testing"
	"time"

	pb "github.com/dunstorm/pm2-go/proto"
)

func TestMetricsPartialObservationPreservesOtherStatusBaselines(t *testing.T) {
	events := newEventStore()
	store := newMetricsStore(events)
	now := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	store.observeSnapshot([]*pb.Process{
		metricProcess(1, "api", "online"),
		metricProcess(2, "worker", "online"),
	})
	store.observePartial([]*pb.Process{
		metricProcess(1, "api", "online"),
	})

	now = now.Add(time.Minute)
	store.observeSnapshot([]*pb.Process{
		metricProcess(1, "api", "online"),
		metricProcess(2, "worker", "stopped"),
	})

	got := events.list()
	if len(got) != 1 {
		t.Fatalf("expected one status event, got %d: %#v", len(got), got)
	}
	if got[0].ProcessID != 2 || got[0].Message != "status changed from online to stopped" {
		t.Fatalf("unexpected status event: %#v", got[0])
	}
}

func TestMetricsSnapshotPrunesDeletedProcessHistory(t *testing.T) {
	events := newEventStore()
	store := newMetricsStore(events)
	now := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	store.observeSnapshot([]*pb.Process{
		metricProcess(1, "api", "online"),
		metricProcess(2, "worker", "online"),
	})
	if got := store.history(2); len(got) != 1 {
		t.Fatalf("expected worker history before delete, got %#v", got)
	}

	now = now.Add(time.Minute)
	store.observeSnapshot([]*pb.Process{
		metricProcess(1, "api", "online"),
	})
	if got := store.history(2); len(got) != 0 {
		t.Fatalf("expected deleted process history to be pruned, got %#v", got)
	}
}

func metricProcess(id int32, name string, status string) *pb.Process {
	return &pb.Process{
		Id:   id,
		Name: name,
		ProcStatus: &pb.ProcStatus{
			Status: status,
			Cpu:    "1.0%",
			Memory: "10.0MB",
		},
	}
}
