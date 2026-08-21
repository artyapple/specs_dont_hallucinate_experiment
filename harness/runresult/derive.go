package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// baselineCaseIDs and contractCaseIDs partition the common ("all") cases so the
// nine common gates can be derived deterministically from evaluator evidence.
var baselineCaseIDs = []string{
	"baseline.create-valid",
	"baseline.create-invalid-title",
	"baseline.get-existing",
	"baseline.get-not-found",
	"baseline.list-ordered",
	"baseline.delete-existing",
	"baseline.delete-again-not-found",
}

// deriveGates maps evaluator setup and case outcomes onto the nine common
// gates required by schemas/run-result.schema.json. The mapping is draft
// until the global freeze and is documented in harness/README.md.
func deriveGates(standalone evaluatorResult, formalOK bool, task string) CommonGates {
	passed := func(ids ...string) bool {
		for _, c := range standalone.BehaviorCases {
			for _, id := range ids {
				if c.ID == id && (!c.Applicable || c.Passed == nil || !*c.Passed) {
					return false
				}
			}
		}
		return true
	}
	baselineOK := passed(baselineCaseIDs...)
	contractsOK := passed("contract.openapi-conformance", "contract.problem-details", "contract.database-consistency")

	taskOK := baselineOK
	if task != TaskBaselineService {
		taskOK = true
		prefix := taskPrefix(task)
		for _, c := range standalone.BehaviorCases {
			if strings.HasPrefix(c.ID, prefix) && c.Applicable && (c.Passed == nil || !*c.Passed) {
				taskOK = false
			}
		}
	}

	return CommonGates{
		FormalInputs:        formalOK,
		Build:               standalone.Setup.Build,
		Migrations:          standalone.Setup.Migrations,
		ServiceStart:        standalone.Setup.Ready,
		BaselineBehavior:    baselineOK,
		TaskBehavior:        taskOK,
		Regressions:         baselineOK && contractsOK,
		APIConformance:      passed("contract.openapi-conformance", "contract.problem-details"),
		DatabaseConsistency: passed("contract.database-consistency"),
	}
}

const TaskBaselineService = "baseline-service"

// taskPrefix maps a run task to its behavior-case ID prefix.
func taskPrefix(task string) string {
	switch task {
	case "nullable-patch":
		return "nullable."
	case "optimistic-locking":
		return "locking."
	case "cursor-pagination":
		return "pagination."
	default:
		return ""
	}
}

// checkFormalInputs implements the draft formal-inputs gate. Propagation-only
// runs must reproduce the frozen target manifest byte-for-byte. Greenfield and
// full-workflow runs may edit formal inputs, so the gate checks their own
// manifest plus basic validity; deeper consistency is proven by the migrations,
// service-start, and behavior gates.
func checkFormalInputs(workspace string, cell Cell, propagationDir string) (bool, string) {
	if cell.Mode == "propagation-only" {
		return checkPropagationTarget(workspace, cell.Task, propagationDir)
	}
	openapiPath := filepath.Join(workspace, "api", "openapi.yaml")
	openapi, err := os.ReadFile(openapiPath)
	if err != nil || len(bytes.TrimSpace(openapi)) == 0 {
		return false, "api/openapi.yaml is missing or empty"
	}
	var document map[string]any
	if err := yaml.Unmarshal(openapi, &document); err != nil {
		return false, "api/openapi.yaml does not parse as YAML: " + err.Error()
	}
	if _, ok := document["openapi"]; !ok {
		return false, "api/openapi.yaml has no openapi key"
	}
	if _, ok := document["paths"]; !ok {
		return false, "api/openapi.yaml has no paths key"
	}
	queries, err := os.ReadFile(filepath.Join(workspace, "db", "queries", "tasks.sql"))
	if err != nil || len(bytes.TrimSpace(queries)) == 0 {
		return false, "db/queries/tasks.sql is missing or empty"
	}
	entries, err := os.ReadDir(filepath.Join(workspace, "db", "migrations"))
	if err != nil {
		return false, "db/migrations is missing"
	}
	migrationPattern := regexp.MustCompile(`^[0-9]{6}_[a-z0-9_]+\.sql$`)
	formalPaths := map[string]bool{"api/openapi.yaml": true, "db/queries/tasks.sql": true}
	for _, entry := range entries {
		if !entry.IsDir() && migrationPattern.MatchString(entry.Name()) {
			formalPaths["db/migrations/"+entry.Name()] = true
		}
	}
	if len(formalPaths) == 2 {
		return false, "db/migrations contains no NNNNNN_name.sql migration"
	}
	return checkFormalManifest(workspace, formalPaths)
}

func checkFormalManifest(workspace string, expected map[string]bool) (bool, string) {
	data, err := os.ReadFile(filepath.Join(workspace, "formal.sha256"))
	if err != nil {
		return false, "formal.sha256 is missing"
	}
	declared := map[string]bool{}
	for index, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != 64 {
			return false, fmt.Sprintf("formal.sha256 line %d is invalid", index+1)
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			return false, fmt.Sprintf("formal.sha256 line %d has an invalid SHA-256", index+1)
		}
		rel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(fields[1])))
		if rel != fields[1] || !expected[rel] || declared[rel] {
			return false, "formal.sha256 has an unexpected or duplicate path: " + fields[1]
		}
		content, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(rel)))
		if err != nil {
			return false, "formal.sha256 path is missing: " + rel
		}
		sum := sha256.Sum256(content)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), fields[0]) {
			return false, "formal.sha256 hash mismatch: " + rel
		}
		declared[rel] = true
	}
	for rel := range expected {
		if !declared[rel] {
			return false, "formal.sha256 does not declare: " + rel
		}
	}
	return true, ""
}

type targetManifest struct {
	Files []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"files"`
}

func checkPropagationTarget(workspace, task, propagationDir string) (bool, string) {
	manifestPath := filepath.Join(propagationDir, task, "target-manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return false, fmt.Sprintf("read %s: %v", manifestPath, err)
	}
	var manifest targetManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return false, fmt.Sprintf("parse %s: %v", manifestPath, err)
	}
	for _, file := range manifest.Files {
		content, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(file.Path)))
		if err != nil {
			return false, "formal target " + file.Path + " is missing"
		}
		sum := sha256.Sum256(content)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), file.SHA256) {
			return false, "formal target " + file.Path + " does not match the frozen target manifest"
		}
	}
	return true, ""
}

// countCandidateTests reports candidate-authored Go test files. Missing tests
// never fail completeSuccess; they are a separate quality finding.
func countCandidateTests(workspace string) CandidateTests {
	var count int64
	_ = filepath.WalkDir(workspace, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() && strings.HasPrefix(entry.Name(), ".") && path != workspace {
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_test.go") {
			count++
		}
		return nil
	})
	return CandidateTests{Present: count > 0, TestFiles: count}
}

// measureCodegenHealth regenerates derived code from the candidate's own
// formal inputs inside the pinned codegen tool image and compares byte-for-byte
// against the committed generated files. The generation commands are
// harness-owned; candidate scripts are never invoked. Draft semantics:
// manualEditDetected is generationSucceeded && !canonical, which conflates
// manual edits with forgotten regeneration until human review.
func measureCodegenHealth(workspace, image string) (*CodegenHealth, error) {
	const script = `set -u
mkdir -p /tmp/work
cp -R /src/. /tmp/work/
cd /tmp/work
FILES="internal/httpapi/generated.gen.go internal/repository/generated/db.go internal/repository/generated/models.go internal/repository/generated/querier.go internal/repository/generated/tasks.sql.go"
gen() {
  oapi-codegen --config oapi-codegen.yaml api/openapi.yaml >internal/httpapi/generated.gen.go && sqlc generate -f sqlc.yaml
}
snap() {
  for f in $FILES; do
    if [ -f "$f" ]; then sha256sum "$f"; else echo "missing  $f"; fi
  done
}
if ! gen >/tmp/gen1.log 2>&1; then
  printf '{"generationSucceeded":false,"canonical":false,"idempotent":false,"manualEditDetected":false}\n'
  exit 0
fi
snap >/tmp/snap1.txt
if gen >/tmp/gen2.log 2>&1 && cmp -s /tmp/snap1.txt <(snap); then idem=true; else idem=false; fi
canonical=true
for f in $FILES; do
  cmp -s "$f" "/src/$f" 2>/dev/null || canonical=false
done
manual=false
[ "$canonical" = true ] || manual=true
printf '{"generationSucceeded":true,"canonical":%s,"idempotent":%s,"manualEditDetected":%s}\n' "$canonical" "$idem" "$manual"
`
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "docker", "run", "--rm",
		"--read-only",
		"--tmpfs", "/tmp:rw,exec,nosuid,nodev,uid=10001,gid=10001",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--network", "none",
		"-e", "HOME=/tmp",
		"-v", workspace+":/src:ro",
		"--entrypoint", "bash",
		image, "-c", script)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("run codegen health container: %w: %s", err, trimEvidence(stderr.String()))
	}
	var health CodegenHealth
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &health); err != nil {
		return nil, fmt.Errorf("parse codegen health output %q: %w", trimEvidence(stdout.String()), err)
	}
	return &health, nil
}
