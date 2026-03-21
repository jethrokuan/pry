package clipboard

import "os"

// readAndRemove reads a file's contents and removes it.
func readAndRemove(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	os.Remove(path)
	return data, err
}

// removeFile removes a file, ignoring errors.
func removeFile(path string) {
	os.Remove(path)
}
