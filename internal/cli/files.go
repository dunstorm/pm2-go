package cli

import (
	"os"
	"path/filepath"
	"strings"
)

func isJSONFilePath(filePath string) bool {
	if !strings.EqualFold(filepath.Ext(filePath), ".json") {
		return false
	}
	info, err := os.Stat(filePath)
	return err == nil && !info.IsDir()
}
