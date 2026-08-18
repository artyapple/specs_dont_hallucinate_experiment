package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func generateSchedule(configPath, phase, seed, revision, generatedAt string) ([]byte, error) {
	if phase != "measured" && phase != "pilot" {
		return nil, fmt.Errorf("phase must be measured or pilot")
	}
	if seed == "" || isTODO(seed) {
		return nil, fmt.Errorf("seed must be explicit and non-TODO")
	}
	if revision == "" || isTODO(revision) {
		return nil, fmt.Errorf("config-revision must be explicit and non-TODO")
	}
	if _, err := time.Parse(time.RFC3339, generatedAt); err != nil {
		return nil, fmt.Errorf("generated-at must be RFC3339: %w", err)
	}
	var config experimentConfig
	if _, err := readJSON(configPath, &config); err != nil {
		return nil, err
	}
	strata, err := matrixStrata(config.Cells)
	if err != nil {
		return nil, err
	}
	blocks := 1
	if phase == "measured" {
		blocks = 5
	}
	stream := newCounterStream(seed, phase)
	runs := make([]scheduleRun, 0, blocks*14)
	for block := 0; block < blocks; block++ {
		order := append([]stratum(nil), strata...)
		stream.shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		for _, stratum := range order {
			pair := [2]cell{stratum.Direct, stratum.Codegen}
			if stream.uintn(2) == 1 {
				pair[0], pair[1] = pair[1], pair[0]
			}
			for _, selected := range pair {
				ordinal := len(runs) + 1
				runs = append(runs, scheduleRun{
					Ordinal: ordinal, RunID: fmt.Sprintf("%s-%03d-%s-r%d", phase, ordinal, selected.ID, block+1),
					CellID: selected.ID, RepeatIndex: block + 1,
				})
			}
		}
	}
	document := schedule{
		Schema: "../schemas/schedule.schema.json", SchemaVersion: 1, Status: "draft", Strategy: "blocked-randomized",
		Algorithm: "sha256-fisher-yates-v1", Seed: seed, GeneratedAt: &generatedAt, ConfigRevision: revision, Runs: runs,
	}
	if err := validateScheduleSemantic(config, document, phase); err != nil {
		return nil, fmt.Errorf("generated schedule is invalid: %w", err)
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

type stratum struct {
	Key             string
	Direct, Codegen cell
}

func matrixStrata(cells []cell) ([]stratum, error) {
	byKey := map[string]*stratum{}
	for _, c := range cells {
		key := c.Stage + "\x00" + c.Task + "\x00" + c.Mode
		s := byKey[key]
		if s == nil {
			s = &stratum{Key: key}
			byKey[key] = s
		}
		switch c.Treatment {
		case "direct":
			if s.Direct.ID != "" {
				return nil, fmt.Errorf("matrix has duplicate direct treatment in stratum %q", key)
			}
			s.Direct = c
		case "codegen":
			if s.Codegen.ID != "" {
				return nil, fmt.Errorf("matrix has duplicate codegen treatment in stratum %q", key)
			}
			s.Codegen = c
		default:
			return nil, fmt.Errorf("matrix cell %q has unsupported treatment %q", c.ID, c.Treatment)
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sortStrings(keys)
	if len(keys) != 7 {
		return nil, fmt.Errorf("matrix must contain seven strata, got %d", len(keys))
	}
	result := make([]stratum, 0, 7)
	for _, key := range keys {
		s := *byKey[key]
		if s.Direct.ID == "" || s.Codegen.ID == "" {
			return nil, fmt.Errorf("stratum %q must contain direct and codegen treatments", key)
		}
		result = append(result, s)
	}
	return result, nil
}

// counterStream is architecture-independent: block N is SHA-256 of
// "freezecheck-schedule-v1\x00", seed, NUL, phase, NUL, and an 8-byte big-endian
// counter. uintn uses rejection sampling; shuffle is Fisher-Yates.
type counterStream struct {
	prefix  []byte
	counter uint64
	buffer  []byte
}

func newCounterStream(seed, phase string) *counterStream {
	return &counterStream{prefix: []byte("freezecheck-schedule-v1\x00" + seed + "\x00" + phase + "\x00")}
}

func (s *counterStream) next64() uint64 {
	if len(s.buffer) < 8 {
		input := make([]byte, len(s.prefix)+8)
		copy(input, s.prefix)
		binary.BigEndian.PutUint64(input[len(s.prefix):], s.counter)
		digest := sha256.Sum256(input)
		s.counter++
		s.buffer = append(s.buffer, digest[:]...)
	}
	value := binary.BigEndian.Uint64(s.buffer[:8])
	s.buffer = s.buffer[8:]
	return value
}

func (s *counterStream) uintn(n uint64) uint64 {
	threshold := -n % n
	for {
		value := s.next64()
		if value >= threshold {
			return value % n
		}
	}
}

func (s *counterStream) shuffle(length int, swap func(int, int)) {
	for i := length - 1; i > 0; i-- {
		swap(i, int(s.uintn(uint64(i+1))))
	}
}

func validateScheduleFiles(configPath, schedulePath, phase string) error {
	var config experimentConfig
	if _, err := readJSON(configPath, &config); err != nil {
		return err
	}
	var document schedule
	data, err := readJSON(schedulePath, &document)
	if err != nil {
		return err
	}
	root := filepath.Dir(filepath.Dir(configPath))
	if err := validateSchema(filepath.Join(root, "schemas", "schedule.schema.json"), data, false); err != nil {
		return err
	}
	return validateScheduleSemantic(config, document, phase)
}

func validateScheduleSemantic(config experimentConfig, document schedule, phase string) error {
	errs := []string{}
	if document.Algorithm != "sha256-fisher-yates-v1" {
		errs = append(errs, "schedule algorithm must be sha256-fisher-yates-v1")
	}
	expectedBlocks := 1
	if phase == "measured" {
		expectedBlocks = 5
	} else if phase != "pilot" {
		return fmt.Errorf("phase must be measured or pilot")
	}
	strata, err := matrixStrata(config.Cells)
	if err != nil {
		return err
	}
	expectedCells := map[string]cell{}
	cellStratum := map[string]string{}
	for _, s := range strata {
		for _, c := range []cell{s.Direct, s.Codegen} {
			expectedCells[c.ID] = c
			cellStratum[c.ID] = s.Key
		}
	}
	if len(document.Runs) != expectedBlocks*14 {
		errs = append(errs, fmt.Sprintf("runs must contain exactly %d entries, got %d", expectedBlocks*14, len(document.Runs)))
	}
	ordinals, runIDs, occurrences := map[int]bool{}, map[string]bool{}, map[string]int{}
	for index, run := range document.Runs {
		if run.Ordinal != index+1 {
			errs = append(errs, fmt.Sprintf("runs[%d].ordinal must be %d, got %d", index, index+1, run.Ordinal))
		}
		if ordinals[run.Ordinal] {
			errs = append(errs, fmt.Sprintf("ordinal %d is duplicated", run.Ordinal))
		}
		ordinals[run.Ordinal] = true
		if runIDs[run.RunID] {
			errs = append(errs, fmt.Sprintf("runId %q is duplicated", run.RunID))
		}
		runIDs[run.RunID] = true
		if _, ok := expectedCells[run.CellID]; !ok {
			errs = append(errs, fmt.Sprintf("runs[%d].cellId %q is not in experiment matrix", index, run.CellID))
		}
		occurrences[fmt.Sprintf("%s\x00%d", run.CellID, run.RepeatIndex)]++
	}
	for id := range expectedCells {
		for repeat := 1; repeat <= expectedBlocks; repeat++ {
			if count := occurrences[fmt.Sprintf("%s\x00%d", id, repeat)]; count != 1 {
				errs = append(errs, fmt.Sprintf("cell %q repeat %d must occur once, got %d", id, repeat, count))
			}
		}
	}
	for block := 0; block < expectedBlocks && block*14+14 <= len(document.Runs); block++ {
		seen := map[string]bool{}
		for pair := 0; pair < 7; pair++ {
			left := document.Runs[block*14+pair*2]
			right := document.Runs[block*14+pair*2+1]
			leftCell, leftOK := expectedCells[left.CellID]
			rightCell, rightOK := expectedCells[right.CellID]
			if !leftOK || !rightOK || cellStratum[left.CellID] != cellStratum[right.CellID] || leftCell.Treatment == rightCell.Treatment {
				errs = append(errs, fmt.Sprintf("block %d pair %d must be one adjacent direct/codegen pair from the same stratum", block+1, pair+1))
			} else if seen[cellStratum[left.CellID]] {
				errs = append(errs, fmt.Sprintf("block %d repeats stratum %q", block+1, cellStratum[left.CellID]))
			} else {
				seen[cellStratum[left.CellID]] = true
			}
			if left.RepeatIndex != block+1 || right.RepeatIndex != block+1 {
				errs = append(errs, fmt.Sprintf("block %d pair %d repeatIndex must be %d", block+1, pair+1, block+1))
			}
		}
		if len(seen) != 7 {
			errs = append(errs, fmt.Sprintf("block %d must contain each of seven strata once", block+1))
		}
	}
	if !isTODO(config.Execution.ScheduleSeed) && document.Seed != config.Execution.ScheduleSeed {
		errs = append(errs, "schedule seed does not match non-TODO experiment scheduleSeed")
	}
	if !isTODO(config.FrozenInputs.ConfigRevision) && document.ConfigRevision != config.FrozenInputs.ConfigRevision {
		errs = append(errs, "schedule configRevision does not match non-TODO experiment configRevision")
	}
	return diagnostics(errs)
}

func writeOutput(path string, data []byte) error {
	if path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	dir := filepath.Dir(path)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("output %s already exists", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	temp, err := os.CreateTemp(dir, ".freezecheck-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	// Link provides atomic publication and, unlike rename, cannot replace a file
	// concurrently created after the existence check.
	if err := os.Link(tempName, path); err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("output %s already exists", path)
		}
		return err
	}
	directory, err := os.Open(dir)
	if err == nil {
		err = directory.Sync()
		_ = directory.Close()
	}
	return err
}
