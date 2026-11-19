package sync

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// loadJSONTarget keeps the exact bytes of a document the ordered parser cannot
// read. Callers must back them up before writing the new document.
func loadJSONTarget(path string) (*mcpObj, []byte, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return newMCPObj(), nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("destination is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	parsed, err := mcpParseJSONOrdered(data)
	if err != nil {
		return newMCPObj(), data, nil
	}
	if obj, ok := parsed.(*mcpObj); ok {
		return obj, nil, nil
	}
	return newMCPObj(), nil, nil
}

func backupInvalidJSON(path string, data []byte, stderr io.Writer) error {
	if data == nil {
		return nil
	}
	backup := path + ".bak"
	for n := 1; ; n++ {
		f, err := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			backup = fmt.Sprintf("%s.bak.%d", path, n)
			continue
		}
		if err != nil {
			return fmt.Errorf("backing up invalid JSON: %w", err)
		}
		_, writeErr := f.Write(data)
		if err := errors.Join(writeErr, f.Close()); err != nil {
			_ = os.Remove(backup)
			return fmt.Errorf("backing up invalid JSON: %w", err)
		}
		fmt.Fprintf(stderr, "warning: invalid JSON in %s; backed up to %s\n", path, backup)
		return nil
	}
}
