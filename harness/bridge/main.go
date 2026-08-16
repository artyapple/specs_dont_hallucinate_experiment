package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	workspace      = "/workspace"
	maxRequestBody = 4 << 20
)

type request struct {
	Tool string          `json:"tool"`
	Args json.RawMessage `json:"args"`
}

type debugEnvelope struct {
	Result json.RawMessage `json:"result"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})
	mux.HandleFunc("POST /execute", execute)

	server := &http.Server{
		Addr:              ":4096",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}

func execute(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input request
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request: %w", err))
		return
	}
	if err := validate(input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	args := []string{"debug", "agent", "experiment", "--tool", input.Tool, "--params", string(input.Args)}
	cmd := exec.CommandContext(context.WithoutCancel(r.Context()), "/usr/local/bin/opencode", args...)
	cmd.Dir = workspace
	debugModel := "openai/gpt-5.6-sol"
	if input.Tool == "edit" || input.Tool == "write" {
		debugModel = "openai/gpt-4.1"
	}
	cmd.Env = append(os.Environ(),
		fmt.Sprintf(`OPENCODE_CONFIG_CONTENT={"model":%q,"formatter":false,"lsp":false,"agent":{"experiment":{"mode":"primary","model":%q,"permission":{"*":"allow"}}}}`, debugModel, debugModel),
		"OPENCODE_DISABLE_PROJECT_CONFIG=1",
		"OPENCODE_DISABLE_DEFAULT_PLUGINS=1",
		"OPENCODE_DISABLE_EXTERNAL_SKILLS=1",
		"OPENCODE_DISABLE_CLAUDE_CODE=1",
		"OPENCODE_DISABLE_LSP_DOWNLOAD=1",
		"XDG_DATA_HOME=/tmp/opencode-data",
		"XDG_CACHE_HOME=/tmp/opencode-cache",
		"XDG_CONFIG_HOME=/tmp/opencode-config",
		"XDG_STATE_HOME=/tmp/opencode-state",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("start native tool: %w", err))
		return
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("native tool failed: %w: %s", err, strings.TrimSpace(stderr.String())))
			return
		}
	case <-r.Context().Done():
		killProcessTree(cmd.Process.Pid)
		<-done
		return
	}

	var envelope debugEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || len(envelope.Result) == 0 {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("invalid native tool response: %w: %s", err, strings.TrimSpace(stdout.String())))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(envelope.Result)
}

func killProcessTree(root int) {
	parents := map[int]int{}
	entries, _ := os.ReadDir("/proc")
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			continue
		}
		end := strings.LastIndexByte(string(data), ')')
		if end < 0 {
			continue
		}
		fields := strings.Fields(string(data[end+1:]))
		if len(fields) < 2 {
			continue
		}
		ppid, err := strconv.Atoi(fields[1])
		if err == nil {
			parents[pid] = ppid
		}
	}

	for {
		killed := false
		for pid, parent := range parents {
			if parent != root {
				continue
			}
			killProcessTree(pid)
			_ = syscall.Kill(pid, syscall.SIGKILL)
			delete(parents, pid)
			killed = true
		}
		if !killed {
			break
		}
	}
	_ = syscall.Kill(-root, syscall.SIGKILL)
	_ = syscall.Kill(root, syscall.SIGKILL)
}

func validate(input request) error {
	switch input.Tool {
	case "read":
		var args struct {
			FilePath string `json:"filePath"`
		}
		if err := json.Unmarshal(input.Args, &args); err != nil {
			return fmt.Errorf("invalid read arguments: %w", err)
		}
		return validateExistingPath(args.FilePath)
	case "edit":
		var args struct {
			FilePath string `json:"filePath"`
		}
		if err := json.Unmarshal(input.Args, &args); err != nil {
			return fmt.Errorf("invalid edit arguments: %w", err)
		}
		path, err := workspacePath(args.FilePath)
		if err != nil {
			return err
		}
		return validateNearestExistingParent(path)
	case "write":
		var args struct {
			FilePath string `json:"filePath"`
		}
		if err := json.Unmarshal(input.Args, &args); err != nil {
			return fmt.Errorf("invalid write arguments: %w", err)
		}
		path, err := workspacePath(args.FilePath)
		if err != nil {
			return err
		}
		return validateNearestExistingParent(path)
	case "bash":
		var args struct {
			Workdir string `json:"workdir"`
		}
		if err := json.Unmarshal(input.Args, &args); err != nil {
			return fmt.Errorf("invalid bash arguments: %w", err)
		}
		if args.Workdir == "" {
			return nil
		}
		return validateExistingPath(args.Workdir)
	case "apply_patch":
		var args struct {
			PatchText string `json:"patchText"`
		}
		if err := json.Unmarshal(input.Args, &args); err != nil {
			return fmt.Errorf("invalid apply_patch arguments: %w", err)
		}
		return validatePatchPaths(args.PatchText)
	default:
		return fmt.Errorf("unsupported tool %q", input.Tool)
	}
}

func validateExistingPath(name string) error {
	path, err := workspacePath(name)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	return requireWorkspacePath(resolved)
}

func validatePatchPaths(patch string) error {
	const prefixes = "*** Add File: \n*** Update File: \n*** Delete File: \n*** Move to: "
	found := false
	for _, line := range strings.Split(strings.ReplaceAll(patch, "\r\n", "\n"), "\n") {
		for _, prefix := range strings.Split(prefixes, "\n") {
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			found = true
			name := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			path, err := workspacePath(name)
			if err != nil {
				return err
			}
			if err := validateNearestExistingParent(path); err != nil {
				return err
			}
		}
	}
	if !found {
		return errors.New("patch contains no file paths")
	}
	return nil
}

func workspacePath(name string) (string, error) {
	if name == "" {
		return "", errors.New("path is required")
	}
	path := name
	if !filepath.IsAbs(path) {
		path = filepath.Join(workspace, path)
	}
	path = filepath.Clean(path)
	if err := requireWorkspacePath(path); err != nil {
		return "", err
	}
	return path, nil
}

func validateNearestExistingParent(path string) error {
	current := path
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			return requireWorkspacePath(resolved)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("resolve path: %w", err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return errors.New("path has no existing parent")
		}
		current = parent
	}
}

func requireWorkspacePath(path string) error {
	rel, err := filepath.Rel(workspace, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q is outside %s", path, workspace)
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
