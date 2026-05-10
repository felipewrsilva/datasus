package storage

import "os"

// PartialSuffix is appended to the output path while conversion is in progress.
const PartialSuffix = ".partial"

// RemovePartialIfExists deletes outPath + ".partial" if present.
// Safe for resume after crash or abrupt exit; ignore missing file.
func RemovePartialIfExists(outPath string) error {
	p := outPath + PartialSuffix
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
