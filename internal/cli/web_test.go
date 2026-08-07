package cli

import (
	"testing"

	"github.com/dunstorm/pm2-go/internal/web"
)

func TestShouldSpawnWebDaemon(t *testing.T) {
	tests := []struct {
		name       string
		noDaemon   bool
		daemonPort int
		want       bool
	}{
		{
			name:       "default port starts daemon",
			daemonPort: web.DefaultDaemonPort,
			want:       true,
		},
		{
			name:       "no daemon flag skips default port",
			noDaemon:   true,
			daemonPort: web.DefaultDaemonPort,
			want:       false,
		},
		{
			name:       "custom port skips default daemon startup",
			daemonPort: web.DefaultDaemonPort + 1,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSpawnWebDaemon(tt.noDaemon, tt.daemonPort)
			if got != tt.want {
				t.Fatalf("expected shouldSpawnWebDaemon(%t, %d) = %t, got %t", tt.noDaemon, tt.daemonPort, tt.want, got)
			}
		})
	}
}
