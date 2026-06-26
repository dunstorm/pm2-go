package cli

func isRunningStatus(status string) bool {
	return status == "online" || status == "unhealthy"
}
