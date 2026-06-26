package web

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "github.com/dunstorm/pm2-go/proto"
)

const metricRetention = time.Hour

type metricPoint struct {
	Timestamp string  `json:"timestamp"`
	CPU       float64 `json:"cpu"`
	MemoryMB  float64 `json:"memory_mb"`
	Status    string  `json:"status"`
}

type metricsStore struct {
	mu      sync.Mutex
	points  map[int32][]metricPoint
	status  map[int32]string
	now     func() time.Time
	events  *eventStore
	retains time.Duration
}

func newMetricsStore(events *eventStore) *metricsStore {
	return &metricsStore{
		points:  make(map[int32][]metricPoint),
		status:  make(map[int32]string),
		now:     time.Now,
		events:  events,
		retains: metricRetention,
	}
}

func (store *metricsStore) observe(processes []*pb.Process) {
	store.mu.Lock()
	defer store.mu.Unlock()

	now := store.now().UTC().Truncate(time.Minute)
	cutoff := now.Add(-store.retains)
	seen := make(map[int32]struct{}, len(processes))

	for _, process := range processes {
		if process == nil {
			continue
		}
		seen[process.Id] = struct{}{}
		status := "unknown"
		cpu := 0.0
		memoryMB := 0.0
		if process.ProcStatus != nil {
			status = normalizeStatus(process.ProcStatus.Status)
			cpu = parsePercent(process.ProcStatus.Cpu)
			memoryMB = parseMemoryMB(process.ProcStatus.Memory)
		}

		if previous, ok := store.status[process.Id]; ok && previous != status {
			store.events.add(eventView{
				Timestamp:   now.Format(time.RFC3339),
				ProcessID:   process.Id,
				ProcessName: process.Name,
				Type:        "status",
				Message:     "status changed from " + previous + " to " + status,
			})
		}
		store.status[process.Id] = status

		point := metricPoint{
			Timestamp: now.Format(time.RFC3339),
			CPU:       cpu,
			MemoryMB:  memoryMB,
			Status:    status,
		}
		store.points[process.Id] = upsertMetricPoint(store.points[process.Id], point, cutoff)
	}

	for id := range store.status {
		if _, ok := seen[id]; !ok {
			delete(store.status, id)
		}
	}
}

func (store *metricsStore) history(id int32) []metricPoint {
	store.mu.Lock()
	defer store.mu.Unlock()

	points := append([]metricPoint(nil), store.points[id]...)
	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp < points[j].Timestamp
	})
	return points
}

func upsertMetricPoint(points []metricPoint, point metricPoint, cutoff time.Time) []metricPoint {
	filtered := points[:0]
	for _, existing := range points {
		timestamp, err := time.Parse(time.RFC3339, existing.Timestamp)
		if err != nil || timestamp.Before(cutoff) {
			continue
		}
		if existing.Timestamp == point.Timestamp {
			continue
		}
		filtered = append(filtered, existing)
	}
	filtered = append(filtered, point)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp < filtered[j].Timestamp
	})
	return filtered
}

func parsePercent(value string) float64 {
	value = strings.TrimSpace(strings.TrimSuffix(value, "%"))
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func parseMemoryMB(value string) float64 {
	value = strings.TrimSpace(strings.ToUpper(value))
	multiplier := 1.0
	switch {
	case strings.HasSuffix(value, "GB"):
		multiplier = 1024
		value = strings.TrimSuffix(value, "GB")
	case strings.HasSuffix(value, "MB"):
		value = strings.TrimSuffix(value, "MB")
	case strings.HasSuffix(value, "KB"):
		multiplier = 1.0 / 1024
		value = strings.TrimSuffix(value, "KB")
	case strings.HasSuffix(value, "B"):
		multiplier = 1.0 / (1024 * 1024)
		value = strings.TrimSuffix(value, "B")
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return parsed * multiplier
}

func normalizeStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return "unknown"
	}
	return status
}
