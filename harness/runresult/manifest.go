package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// workspaceManifest is the draft workspace-manifest.json format. It lets the
// semantic validator prove that preserved workspace files match their recorded
// hashes. Paths are workspace-relative, slash-separated, and sorted.
type workspaceManifest struct {
	SchemaVersion int             `json:"schemaVersion"`
	Files         []manifestEntry `json:"files"`
}

type manifestEntry struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
}

// writeWorkspaceManifest hashes every regular file in the preserved candidate
// workspace. A missing workspace (possible for early harness failures)
// produces an empty manifest rather than an error.
func writeWorkspaceManifest(workspaceDir, outputPath string) error {
	manifest := workspaceManifest{SchemaVersion: 1, Files: []manifestEntry{}}
	if info, err := os.Stat(workspaceDir); err == nil && info.IsDir() {
		err := filepath.WalkDir(workspaceDir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if strings.HasPrefix(entry.Name(), ".") && path != workspaceDir {
					return filepath.SkipDir
				}
				return nil
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			rel, err := filepath.Rel(workspaceDir, path)
			if err != nil {
				return err
			}
			sum, size, err := hashFile(path)
			if err != nil {
				return err
			}
			manifest.Files = append(manifest.Files, manifestEntry{
				Path:      filepath.ToSlash(rel),
				SHA256:    sum,
				SizeBytes: size,
			})
			return nil
		})
		if err != nil {
			return fmt.Errorf("walk workspace: %w", err)
		}
	}
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, append(encoded, '\n'), 0o644)
}

func hashFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), size, nil
}
