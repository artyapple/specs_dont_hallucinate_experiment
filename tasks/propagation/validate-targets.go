package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type targetManifest struct {
	Schema            string       `json:"$schema"`
	SchemaVersion     int          `json:"schemaVersion"`
	Status            string       `json:"status"`
	FormalPatchSHA256 string       `json:"formalPatchSha256"`
	Files             []targetFile `json:"files"`
}

type targetFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

func main() {
	tracks := []string{"nullable-patch", "optimistic-locking", "cursor-pagination"}
	for _, track := range tracks {
		if err := validateTrack(track); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	fmt.Println("canonical task targets validated")
}

func validateTrack(track string) error {
	directory := filepath.Join("tasks", "propagation", track)
	manifestData, err := os.ReadFile(filepath.Join(directory, "target-manifest.json"))
	if err != nil {
		return fmt.Errorf("%s: read manifest: %w", track, err)
	}
	var manifest targetManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("%s: decode manifest: %w", track, err)
	}
	if manifest.Schema != "../../../schemas/target-manifest.schema.json" || manifest.SchemaVersion != 1 || manifest.Status != "draft" || len(manifest.Files) == 0 {
		return fmt.Errorf("%s: invalid manifest metadata", track)
	}
	patchData, err := os.ReadFile(filepath.Join(directory, "formal.patch"))
	if err != nil {
		return fmt.Errorf("%s: read formal patch: %w", track, err)
	}
	if digest(patchData) != manifest.FormalPatchSHA256 {
		return fmt.Errorf("%s: formal patch hash mismatch", track)
	}

	direct := filepath.Join("fixtures", "task-solutions", track+"-direct")
	codegen := filepath.Join("fixtures", "task-solutions", track+"-codegen")
	seen := make(map[string]bool)
	for _, file := range manifest.Files {
		if seen[file.Path] || !validFormalPath(file.Path) {
			return fmt.Errorf("%s: duplicate or invalid formal path %q", track, file.Path)
		}
		seen[file.Path] = true
		directData, err := os.ReadFile(filepath.Join(direct, filepath.FromSlash(file.Path)))
		if err != nil {
			return fmt.Errorf("%s Direct %s: %w", track, file.Path, err)
		}
		codegenData, err := os.ReadFile(filepath.Join(codegen, filepath.FromSlash(file.Path)))
		if err != nil {
			return fmt.Errorf("%s Codegen %s: %w", track, file.Path, err)
		}
		if !bytes.Equal(directData, codegenData) {
			return fmt.Errorf("%s: %s differs between Direct and Codegen", track, file.Path)
		}
		if digest(directData) != file.SHA256 {
			return fmt.Errorf("%s: %s hash mismatch", track, file.Path)
		}
	}
	return nil
}

func validFormalPath(path string) bool {
	return path == "api/openapi.yaml" ||
		path == "db/queries/tasks.sql" ||
		filepath.Dir(filepath.FromSlash(path)) == filepath.Join("db", "migrations")
}

func digest(data []byte) string {
	value := sha256.Sum256(data)
	return hex.EncodeToString(value[:])
}
