package projects

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"fabrica/backend/internal/securitygateway"
)

func specificationContainsScopeArtifact(rawPaths []byte, taskID int64) bool {
	var paths []string
	if json.Unmarshal(rawPaths, &paths) != nil {
		return false
	}
	wanted := filepath.ToSlash(filepath.Join("openspec", "changes", fmt.Sprintf("task-%d", taskID), "scope.json"))
	for _, candidate := range paths {
		if candidate == wanted {
			return true
		}
	}
	return false
}

func readTaskWriteScope(projectPath string, taskID int64) ([]string, error) {
	projectRoot, err := filepath.EvalSymlinks(projectPath)
	if err != nil {
		return nil, errors.New("project workspace is unavailable")
	}
	relative := filepath.Join("openspec", "changes", fmt.Sprintf("task-%d", taskID), "scope.json")
	path := filepath.Join(projectRoot, relative)
	current := projectRoot
	for _, component := range []string{"openspec", "changes", fmt.Sprintf("task-%d", taskID), "scope.json"} {
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("scope manifest is unavailable or unsafe")
		}
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<10 {
		return nil, errors.New("scope manifest is unavailable or exceeds the size limit")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("scope manifest could not be read")
	}
	paths, err := securitygateway.ParseScopeManifest(data, taskID)
	if err != nil {
		return nil, err
	}
	return paths, nil
}
