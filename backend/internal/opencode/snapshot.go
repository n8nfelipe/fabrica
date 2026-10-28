package opencode

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Snapshot map[string][32]byte

var ignoredDirectories = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".venv": true, ".cache": true,
}

func CaptureSnapshot(root string) (Snapshot, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve snapshot root: %w", err)
	}
	files := Snapshot{}
	var totalBytes int64
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != root && entry.IsDir() && ignoredDirectories[entry.Name()] {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		totalBytes += info.Size()
		if totalBytes > 512<<20 {
			return fmt.Errorf("project snapshot exceeds the 512 MiB scan limit")
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		hasher := sha256.New()
		_, copyErr := io.Copy(hasher, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		var digest [32]byte
		copy(digest[:], hasher.Sum(nil))
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = digest
		if len(files) > 100000 {
			return fmt.Errorf("project contains too many files to snapshot")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func ChangedFiles(before, after Snapshot) []string {
	changed := make([]string, 0)
	for path, digest := range before {
		if current, exists := after[path]; !exists || current != digest {
			changed = append(changed, path)
		}
	}
	for path := range after {
		if _, existed := before[path]; !existed {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed
}

func IsWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}
