package workeradapter

import (
	"os"
	"path/filepath"
)

// GetDataDir returns the data directory from the environment or a default.
func GetDataDir() string {
	if dir := os.Getenv("HOSTEDAT_DATA_DIR"); dir != "" {
		return dir
	}
	return "./data"
}

// GetD1Path returns the filesystem path for a D1 database file.
func GetD1Path(dataDir, databaseID string) string {
	return filepath.Join(dataDir, "d1", databaseID+".sqlite3")
}

// DeleteFile removes a file from the filesystem.
func DeleteFile(path string) error {
	return os.Remove(path)
}
