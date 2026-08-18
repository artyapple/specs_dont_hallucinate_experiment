package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type versionsFile struct {
	Status string            `json:"status"`
	Frozen map[string]string `json:"frozen"`
}

type designRevisionFile struct {
	SchemaVersion int    `json:"schemaVersion"`
	Status        string `json:"status"`
	Source        string `json:"source"`
	SourceSHA256  string `json:"sourceSha256"`
}

func validateConfig(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	configPath := filepath.Join(root, "config", "experiment.json")
	var config experimentConfig
	configData, err := readJSON(configPath, &config)
	if err != nil {
		return err
	}
	errs := []string{}
	if err := validateSchema(filepath.Join(root, "schemas", "experiment-config.schema.json"), configData, true); err != nil {
		errs = append(errs, "experiment config: "+err.Error())
	}
	var versions versionsFile
	versionsData, err := readJSON(filepath.Join(root, "config", "versions.json"), &versions)
	if err != nil {
		return err
	}
	var design designRevisionFile
	designPath := filepath.Join(root, "config", "design-revision.json")
	if _, err := readJSON(designPath, &design); err != nil {
		errs = append(errs, "design revision: "+err.Error())
	} else {
		if design.SchemaVersion != 1 || (design.Status != "draft" && design.Status != "frozen") {
			errs = append(errs, "design revision must have schemaVersion 1 and draft or frozen status")
		}
		sourcePath := filepath.Clean(filepath.Join(filepath.Dir(designPath), filepath.FromSlash(design.Source)))
		sourceData, readErr := os.ReadFile(sourcePath)
		if readErr != nil {
			errs = append(errs, "design revision source: "+readErr.Error())
		} else {
			sum := sha256.Sum256(sourceData)
			if design.SourceSHA256 != hex.EncodeToString(sum[:]) {
				errs = append(errs, "design revision sourceSha256 does not match the authoritative design source")
			}
		}
	}
	schedulePath, pathErr := containedPath(root, config.FrozenInputs.ScheduleManifest)
	var manifest schedule
	var scheduleData []byte
	if pathErr == nil {
		scheduleData, err = readJSON(schedulePath, &manifest)
		if err == nil {
			err = validateSchema(filepath.Join(root, "schemas", "schedule.schema.json"), scheduleData, false)
		}
	}
	if config.Model.Provider != "openrouter" {
		errs = append(errs, `model.provider must be "openrouter"`)
	}
	if config.Agent.Name != "opencode" {
		errs = append(errs, `agent.name must be "opencode" to match versions.frozen.opencode`)
	} else if want := versions.Frozen["opencode"]; want == "" || config.Agent.Version != want {
		errs = append(errs, fmt.Sprintf("agent.version %q does not match versions.frozen.opencode %q", config.Agent.Version, want))
	}
	imageKeys := map[string]string{"coordinator": "coordinatorImage", "toolDirect": "toolDirectImage", "toolCodegen": "toolCodegenImage", "evaluator": "evaluatorImage"}
	for configKey, versionKey := range imageKeys {
		if config.FrozenInputs.EnvironmentImages[configKey] != versions.Frozen[versionKey] {
			errs = append(errs, fmt.Sprintf("frozenInputs.environmentImages.%s does not match versions.frozen.%s", configKey, versionKey))
		}
	}
	if pathErr != nil {
		errs = append(errs, "schedule manifest: "+pathErr.Error())
	} else if err != nil {
		errs = append(errs, "schedule manifest: "+err.Error())
	} else {
		if manifest.Strategy != config.Execution.Schedule {
			errs = append(errs, "schedule strategy does not match experiment execution.schedule")
		}
		if !isTODO(config.Execution.ScheduleSeed) && !isTODO(manifest.Seed) && manifest.Seed != config.Execution.ScheduleSeed {
			errs = append(errs, "schedule seed does not match experiment execution.scheduleSeed")
		}
		if !isTODO(config.FrozenInputs.ConfigRevision) && !isTODO(manifest.ConfigRevision) && manifest.ConfigRevision != config.FrozenInputs.ConfigRevision {
			errs = append(errs, "schedule configRevision does not match frozenInputs.configRevision")
		}
	}
	if config.Status == "frozen" {
		if design.Status != "frozen" {
			errs = append(errs, `design revision status must be "frozen"`)
		}
		errs = append(errs, frozenConfigErrors(root, configData, versionsData, config, versions, manifest)...)
	}
	return diagnostics(errs)
}

func frozenConfigErrors(root string, configData, versionsData []byte, config experimentConfig, versions versionsFile, manifest schedule) []string {
	errs := []string{}
	for label, data := range map[string][]byte{"experiment config": configData, "versions metadata": versionsData} {
		var value any
		_ = json.Unmarshal(data, &value)
		walkStrings(value, "$", func(path, text string) {
			if isTODO(text) {
				errs = append(errs, fmt.Sprintf("%s %s contains TODO value %q", label, path, text))
			}
		})
	}
	if versions.Status != "frozen" {
		errs = append(errs, `versions.status must be "frozen"`)
	}
	if config.Execution.NetworkPolicyEnforcementStatus != "validated" {
		errs = append(errs, `execution.networkPolicyEnforcementStatus must be "validated"`)
	}
	if manifest.Status != "frozen" {
		errs = append(errs, `schedule status must be "frozen"`)
	} else if err := validateScheduleSemantic(config, manifest, "measured"); err != nil {
		errs = append(errs, "frozen schedule: "+err.Error())
	}
	digest := regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	for key, value := range config.FrozenInputs.EnvironmentImages {
		if !digest.MatchString(value) {
			errs = append(errs, fmt.Sprintf("environment image %s must have sha256:<64 lowercase hex> digest shape", key))
		}
	}
	revisions := map[string]string{
		"configRevision": config.FrozenInputs.ConfigRevision,
		"designRevision": config.FrozenInputs.DesignRevision, "evaluatorRevision": config.FrozenInputs.EvaluatorRevision,
		"resultSchemaRevision": config.FrozenInputs.ResultSchemaRevision,
	}
	for key, value := range config.FrozenInputs.Fixtures {
		revisions["fixtures."+key] = value
	}
	for key, value := range config.FrozenInputs.Treatments {
		revisions["treatments."+key] = value
	}
	for key, revision := range revisions {
		command := exec.Command("git", "-C", root, "cat-file", "-e", revision+"^{commit}")
		if command.Run() != nil {
			errs = append(errs, fmt.Sprintf("Git revision %s=%q does not resolve to a commit", key, revision))
		}
	}
	taskPaths := map[string]string{
		"part1": "tasks/part1.md", "nullablePatchFull": "tasks/full/nullable-patch.md",
		"optimisticLockingFull": "tasks/full/optimistic-locking.md", "cursorPaginationFull": "tasks/full/cursor-pagination.md",
		"nullablePatchPropagationTask": "tasks/propagation/nullable-patch.md", "optimisticLockingPropagationTask": "tasks/propagation/optimistic-locking.md",
		"cursorPaginationPropagationTask": "tasks/propagation/cursor-pagination.md", "nullablePatchFormalPatch": "tasks/propagation/nullable-patch/formal.patch",
		"optimisticLockingFormalPatch": "tasks/propagation/optimistic-locking/formal.patch", "cursorPaginationFormalPatch": "tasks/propagation/cursor-pagination/formal.patch",
	}
	for key, path := range taskPaths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			errs = append(errs, fmt.Sprintf("task input %s cannot be read: %v", path, err))
			continue
		}
		sum := sha256.Sum256(data)
		if !strings.EqualFold(config.FrozenInputs.Tasks[key], hex.EncodeToString(sum[:])) {
			errs = append(errs, fmt.Sprintf("frozenInputs.tasks.%s does not match SHA-256 of %s", key, path))
		}
	}
	status := exec.Command("git", "-C", root, "status", "--porcelain", "--untracked-files=all")
	output, err := status.Output()
	if err != nil {
		errs = append(errs, "cannot inspect Git worktree cleanliness")
	} else if len(output) != 0 {
		errs = append(errs, "repository worktree is not clean")
	}
	return errs
}

func walkStrings(value any, path string, visit func(string, string)) {
	switch value := value.(type) {
	case string:
		visit(path, value)
	case []any:
		for i, child := range value {
			walkStrings(child, fmt.Sprintf("%s[%d]", path, i), visit)
		}
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sortStrings(keys)
		for _, key := range keys {
			walkStrings(value[key], path+"."+key, visit)
		}
	}
}

func containedPath(base, name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("path %q must be relative", name)
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes repository", name)
	}
	return filepath.Join(base, clean), nil
}
