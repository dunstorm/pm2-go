package server

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	pb "github.com/dunstorm/pm2-go/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	defaultHealthCheckInterval = 5 * time.Second
	defaultHealthCheckTimeout  = time.Second
	defaultWatchInterval       = time.Second
)

func isRunningState(status string) bool {
	return status == "online" || status == "unhealthy"
}

func dueSince(last *timestamppb.Timestamp, interval time.Duration) bool {
	return last == nil || time.Since(last.AsTime()) >= interval
}

func healthCheckInterval(p *pb.Process) time.Duration {
	if p.HealthCheckIntervalMs > 0 {
		return time.Duration(p.HealthCheckIntervalMs) * time.Millisecond
	}
	return defaultHealthCheckInterval
}

func healthCheckTimeout(p *pb.Process) time.Duration {
	if p.HealthCheckTimeoutMs > 0 {
		return time.Duration(p.HealthCheckTimeoutMs) * time.Millisecond
	}
	return defaultHealthCheckTimeout
}

func handleHealthCheck(handler *Handler, p *pb.Process) bool {
	if p.HealthCheckUrl == "" || !dueSince(p.LastHealthCheckAt, healthCheckInterval(p)) {
		return false
	}

	p.LastHealthCheckAt = timestamppb.New(time.Now())
	client := http.Client{Timeout: healthCheckTimeout(p)}
	resp, err := client.Get(p.HealthCheckUrl)
	healthy := err == nil && resp.StatusCode >= 200 && resp.StatusCode < 400
	if resp != nil {
		resp.Body.Close()
	}

	if healthy {
		if p.ProcStatus.Status == "unhealthy" {
			handler.logger.Info().Msgf("Process %s health check recovered", p.Name)
		}
		p.UpdateStatus("online")
		return true
	}

	if p.ProcStatus.Status != "unhealthy" {
		handler.logger.Warn().Msgf("Process %s health check failed", p.Name)
	}
	p.UpdateStatus("unhealthy")
	return true
}

func watchInterval(p *pb.Process) time.Duration {
	if p.WatchIntervalMs > 0 {
		return time.Duration(p.WatchIntervalMs) * time.Millisecond
	}
	return defaultWatchInterval
}

func watchedPaths(p *pb.Process) []string {
	if len(p.WatchPaths) > 0 {
		return p.WatchPaths
	}
	return []string{p.Cwd}
}

func resolveWatchPath(p *pb.Process, watchPath string) string {
	if filepath.IsAbs(watchPath) {
		return watchPath
	}
	return filepath.Join(p.Cwd, watchPath)
}

func watchSignature(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.ModTime().UnixNano() + info.Size(), nil
}

func handleWatch(handler *Handler, p *pb.Process) bool {
	if !p.Watch || !dueSince(p.LastWatchCheckAt, watchInterval(p)) {
		return false
	}

	p.LastWatchCheckAt = timestamppb.New(time.Now())
	if p.WatchSignatures == nil {
		p.WatchSignatures = make(map[string]int64)
	}

	changed := false
	for _, watchPath := range watchedPaths(p) {
		resolved := resolveWatchPath(p, watchPath)
		signature, err := watchSignature(resolved)
		if err != nil {
			continue
		}
		if previous, exists := p.WatchSignatures[resolved]; exists && previous != signature {
			changed = true
		}
		p.WatchSignatures[resolved] = signature
	}

	if !changed {
		return false
	}

	handler.logger.Info().Msgf("Watched files changed for process %s", p.Name)
	return true
}
