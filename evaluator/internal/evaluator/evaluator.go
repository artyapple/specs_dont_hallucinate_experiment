package evaluator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const postgresImage = "docker.io/library/postgres:18.6@sha256:06cad38a5d9f5d24b4d83d86def30795d5e4b757fedbf5281172b576dedcd941"

// Frozen resource and diagnostics limits. See contract.md for the full binary
// contract; these values are part of it.
const (
	// EvaluationBudget bounds one complete evaluator run. cmd/evaluator
	// enforces it and aborts without writing a result when it is exceeded.
	EvaluationBudget = 15 * time.Minute
	// candidateCommandBudget bounds one candidate build or migration command.
	candidateCommandBudget = 5 * time.Minute
	// evidenceLogLimit bounds candidate command output and service logs
	// embedded in the result; only the tail is preserved.
	evidenceLogLimit = 16 * 1024
	// responseBodyEvidenceLimit bounds response bodies embedded in case evidence.
	responseBodyEvidenceLimit = 8 * 1024
	// responseBodyDecodeLimit bounds buffered response bodies that are later decoded.
	responseBodyDecodeLimit = 64 * 1024
)

const (
	TaskBaseline   = "baseline-service"
	TaskNullable   = "nullable-patch"
	TaskLocking    = "optimistic-locking"
	TaskPagination = "cursor-pagination"
	taskAll        = "all"
)

type Options struct {
	Candidate string
	Task      string
}

func ValidTask(task string) bool {
	switch task {
	case TaskBaseline, TaskNullable, TaskLocking, TaskPagination:
		return true
	default:
		return false
	}
}

type Result struct {
	SchemaVersion   int          `json:"schemaVersion"`
	Candidate       string       `json:"candidate"`
	Task            string       `json:"task"`
	StartedAt       time.Time    `json:"startedAt"`
	FinishedAt      time.Time    `json:"finishedAt"`
	CompleteSuccess bool         `json:"completeSuccess"`
	Setup           SetupResult  `json:"setup"`
	BehaviorCases   []CaseResult `json:"behaviorCases"`
	ServiceLogs     string       `json:"serviceLogs,omitempty"`
}

type SetupResult struct {
	Postgres bool   `json:"postgres"`
	Build    bool   `json:"build"`
	Migrate  bool   `json:"migrations"`
	Ready    bool   `json:"serviceReady"`
	Evidence string `json:"evidence,omitempty"`
}

type CaseResult struct {
	ID         string `json:"id"`
	Applicable bool   `json:"applicable"`
	Passed     *bool  `json:"passed"`
	Evidence   string `json:"evidence"`
}

type task struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	CreatedAt string          `json:"createdAt"`
	DueAt     json.RawMessage `json:"dueAt"`
	Version   *int64          `json:"version"`
}

type taskList struct {
	Items      []task          `json:"items"`
	NextCursor json.RawMessage `json:"nextCursor"`
}

type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

type suite struct {
	baseURL   string
	client    *http.Client
	db        *pgxpool.Pool
	task      string
	createdID string
}

func Evaluate(ctx context.Context, options Options) (result Result) {
	definitions := caseDefinitions()
	result = Result{SchemaVersion: 1, Candidate: options.Candidate, Task: options.Task, StartedAt: time.Now().UTC()}
	defer func() { result.FinishedAt = time.Now().UTC() }()
	failSetup := func(err error) Result {
		result.Setup.Evidence = err.Error()
		result.BehaviorCases = setupFailureCases(definitions, options.Task, err)
		return result
	}
	if !ValidTask(options.Task) {
		return failSetup(fmt.Errorf("unsupported task %q", options.Task))
	}

	container, err := postgres.Run(ctx, postgresImage,
		postgres.WithDatabase("tasks"),
		postgres.WithUsername("evaluator"),
		postgres.WithPassword("evaluator"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	defer func() {
		if container != nil {
			_ = testcontainers.TerminateContainer(container)
		}
	}()
	if err != nil {
		return failSetup(fmt.Errorf("start PostgreSQL: %w", err))
	}
	result.Setup.Postgres = true
	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return failSetup(fmt.Errorf("get PostgreSQL connection string: %w", err))
	}

	if output, err := runCommand(ctx, options.Candidate, databaseURL, "make", "build"); err != nil {
		return failSetup(fmt.Errorf("build candidate: %w: %s", err, output))
	}
	result.Setup.Build = true
	if output, err := runCommand(ctx, options.Candidate, databaseURL, "make", "migrate"); err != nil {
		return failSetup(fmt.Errorf("apply candidate migrations: %w: %s", err, output))
	}
	result.Setup.Migrate = true

	addr, err := availableAddress()
	if err != nil {
		return failSetup(fmt.Errorf("allocate HTTP address: %w", err))
	}
	service, err := startService(options.Candidate, databaseURL, addr)
	if err != nil {
		return failSetup(fmt.Errorf("start service: %w", err))
	}
	defer service.stop()
	if err := service.waitReady(ctx, "http://"+addr+"/healthz"); err != nil {
		result.ServiceLogs = service.logs()
		return failSetup(fmt.Errorf("wait for service readiness: %w", err))
	}
	result.Setup.Ready = true

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return failSetup(fmt.Errorf("open evaluator database connection: %w", err))
	}
	defer pool.Close()
	s := &suite{
		baseURL: "http://" + addr,
		client:  &http.Client{Timeout: 5 * time.Second},
		db:      pool,
		task:    options.Task,
	}
	result.CompleteSuccess = true
	for _, definition := range definitions {
		applicable := definition.Task == taskAll || definition.Task == options.Task
		caseResult := CaseResult{ID: definition.ID, Applicable: applicable}
		if !applicable {
			result.BehaviorCases = append(result.BehaviorCases, caseResult)
			continue
		}
		if definition.Task != taskAll {
			if err := s.reset(ctx); err != nil {
				caseResult.Passed = boolPointer(false)
				caseResult.Evidence = "reset database: " + err.Error()
				result.CompleteSuccess = false
				result.BehaviorCases = append(result.BehaviorCases, caseResult)
				continue
			}
		}
		err := definition.Run(s, ctx)
		caseResult.Passed = boolPointer(err == nil)
		caseResult.Evidence = "passed"
		if err != nil {
			caseResult.Evidence = err.Error()
			result.CompleteSuccess = false
		}
		result.BehaviorCases = append(result.BehaviorCases, caseResult)
	}
	result.ServiceLogs = service.logs()
	return result
}

func boolPointer(value bool) *bool {
	return &value
}

func setupFailureCases(definitions []caseDefinition, selectedTask string, setupErr error) []CaseResult {
	results := make([]CaseResult, 0, len(definitions))
	for _, definition := range definitions {
		applicable := definition.Task == taskAll || definition.Task == selectedTask
		caseResult := CaseResult{ID: definition.ID, Applicable: applicable}
		if applicable {
			caseResult.Passed = boolPointer(false)
			caseResult.Evidence = "not run: " + setupErr.Error()
		}
		results = append(results, caseResult)
	}
	return results
}

func (s *suite) reset(ctx context.Context) error {
	_, err := s.db.Exec(ctx, "TRUNCATE TABLE tasks")
	return err
}

// providerCredentialVariables are stripped from every candidate command
// environment so candidate-controlled code can never observe provider keys,
// even if the evaluator process environment was populated carelessly.
var providerCredentialVariables = []string{"OPENROUTER_API_KEY"}

// candidateEnvironment returns the evaluator process environment without
// provider credentials, plus the given extra variables.
func candidateEnvironment(extra ...string) []string {
	env := make([]string, 0, len(extra))
	for _, entry := range os.Environ() {
		redacted := false
		for _, key := range providerCredentialVariables {
			if strings.HasPrefix(entry, key+"=") {
				redacted = true
				break
			}
		}
		if !redacted {
			env = append(env, entry)
		}
	}
	return append(env, extra...)
}

func runCommand(ctx context.Context, dir, databaseURL, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, candidateCommandBudget)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.Env = candidateEnvironment("DATABASE_URL=" + databaseURL)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Abort kills the whole candidate command process group, not only the
	// direct child, so candidate toolchains cannot leak grandchildren.
	command.Cancel = func() error {
		if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		return nil
	}
	command.WaitDelay = 5 * time.Second
	output, err := command.CombinedOutput()
	return tailEvidence(string(output), evidenceLogLimit), err
}

// tailEvidence keeps the last limit bytes of candidate-produced text so setup
// and case evidence stay bounded regardless of candidate output volume.
func tailEvidence(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) > limit {
		return fmt.Sprintf("(truncated to last %d bytes)\n%s", limit, trimmed[len(trimmed)-limit:])
	}
	return trimmed
}

// readEvidence reads at most limit bytes of a response body for evidence
// strings, marking truncation explicitly.
func readEvidence(reader io.Reader, limit int64) string {
	data, _ := io.ReadAll(io.LimitReader(reader, limit+1))
	if int64(len(data)) > limit {
		return strings.TrimSpace(string(data[:limit])) + " (truncated)"
	}
	return strings.TrimSpace(string(data))
}

func availableAddress() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	addr := listener.Addr().String()
	return addr, listener.Close()
}

type serviceProcess struct {
	command *exec.Cmd
	done    chan error
	logFile *os.File
	logPath string
}

func startService(dir, databaseURL, addr string) (*serviceProcess, error) {
	logFile, err := os.CreateTemp("", "baseline-evaluator-service-*.log")
	if err != nil {
		return nil, err
	}
	command := exec.Command("make", "run")
	command.Dir = dir
	command.Env = candidateEnvironment("DATABASE_URL="+databaseURL, "HTTP_ADDR="+addr)
	command.Stdout = logFile
	command.Stderr = logFile
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		logFile.Close()
		os.Remove(logFile.Name())
		return nil, err
	}
	process := &serviceProcess{command: command, done: make(chan error, 1), logFile: logFile, logPath: logFile.Name()}
	go func() { process.done <- command.Wait() }()
	return process, nil
}

func (p *serviceProcess) waitReady(ctx context.Context, url string) error {
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if response, err := client.Do(request); err == nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case err := <-p.done:
			return fmt.Errorf("service exited before readiness: %w; logs: %s", err, p.logs())
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("30 second readiness timeout; logs: %s", p.logs())
		case <-ticker.C:
		}
	}
}

func (p *serviceProcess) stop() {
	if p.command.Process != nil {
		_ = syscall.Kill(-p.command.Process.Pid, syscall.SIGTERM)
		select {
		case <-p.done:
		case <-time.After(10 * time.Second):
			_ = syscall.Kill(-p.command.Process.Pid, syscall.SIGKILL)
			<-p.done
		}
	}
	p.logFile.Close()
	os.Remove(p.logPath)
}

func (p *serviceProcess) logs() string {
	data, err := os.ReadFile(p.logPath)
	if err != nil {
		return ""
	}
	return tailEvidence(string(data), evidenceLogLimit)
}

func (s *suite) createValid(ctx context.Context) error {
	response, err := s.jsonRequest(ctx, http.MethodPost, "/tasks", `{"title":"  Привет task  "}`)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := expectStatusAndType(response, http.StatusCreated, "application/json"); err != nil {
		return err
	}
	created, err := s.decodeTask(response.Body)
	if err != nil {
		return err
	}
	if created.Title != "Привет task" {
		return fmt.Errorf("title = %q, want trimmed Unicode title", created.Title)
	}
	if !isUUIDv4(created.ID) {
		return fmt.Errorf("id %q is not UUIDv4", created.ID)
	}
	if !isCanonicalTimestamp(created.CreatedAt) {
		return fmt.Errorf("createdAt %q is not UTC with six fractional digits", created.CreatedAt)
	}
	s.createdID = created.ID
	return nil
}

func (s *suite) createInvalidTitle(ctx context.Context) error {
	for name, body := range map[string]string{
		"blank":    `{"title":" \t\n "}`,
		"too-long": `{"title":"` + strings.Repeat("界", 201) + `"}`,
	} {
		response, err := s.jsonRequest(ctx, http.MethodPost, "/tasks", body)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := expectProblemResponse(response, validationProblem()); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func (s *suite) getExisting(ctx context.Context) error {
	if s.createdID == "" {
		return errors.New("valid create did not produce an id")
	}
	response, err := s.jsonRequest(ctx, http.MethodGet, "/tasks/"+s.createdID, "")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := expectStatusAndType(response, http.StatusOK, "application/json"); err != nil {
		return err
	}
	got, err := s.decodeTask(response.Body)
	if err != nil {
		return err
	}
	if got.ID != s.createdID || got.Title != "Привет task" {
		return fmt.Errorf("unexpected task: %+v", got)
	}
	return nil
}

func (s *suite) getNotFound(ctx context.Context) error {
	response, err := s.jsonRequest(ctx, http.MethodGet, "/tasks/ffffffff-ffff-4fff-8fff-ffffffffffff", "")
	if err != nil {
		return err
	}
	return expectProblemResponse(response, notFoundProblem())
}

func (s *suite) listOrdered(ctx context.Context) error {
	rows := []task{
		{ID: "00000000-0000-4000-8000-000000000002", Title: "tie two", CreatedAt: "2000-01-01T00:00:00.000000Z"},
		{ID: "00000000-0000-4000-8000-000000000001", Title: "tie one", CreatedAt: "2000-01-01T00:00:00.000000Z"},
	}
	for _, row := range rows {
		if _, err := s.db.Exec(ctx, "INSERT INTO tasks (id, title, created_at) VALUES ($1, $2, $3)", row.ID, row.Title, row.CreatedAt); err != nil {
			return fmt.Errorf("seed ordered list: %w", err)
		}
	}
	response, err := s.jsonRequest(ctx, http.MethodGet, "/tasks", "")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := expectStatusAndType(response, http.StatusOK, "application/json"); err != nil {
		return err
	}
	list, err := s.decodeTaskList(response.Body)
	if err != nil {
		return err
	}
	if len(list.Items) != 3 {
		return fmt.Errorf("list has %d items, want 3", len(list.Items))
	}
	if err := checkTaskIDs(list.Items, []string{rows[1].ID, rows[0].ID, s.createdID}); err != nil {
		return fmt.Errorf("ids are not ordered by (createdAt, id): %w", err)
	}
	return nil
}

func (s *suite) deleteExisting(ctx context.Context) error {
	if s.createdID == "" {
		return errors.New("valid create did not produce an id")
	}
	response, err := s.jsonRequest(ctx, http.MethodDelete, "/tasks/"+s.createdID, "")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("status = %d, want 204", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2))
	if err != nil {
		return err
	}
	if len(body) != 0 {
		return fmt.Errorf("204 response has a non-empty body: %s", readEvidence(bytes.NewReader(body), responseBodyEvidenceLimit))
	}
	return nil
}

func (s *suite) deleteAgainNotFound(ctx context.Context) error {
	if s.createdID == "" {
		return errors.New("valid create did not produce an id")
	}
	response, err := s.jsonRequest(ctx, http.MethodDelete, "/tasks/"+s.createdID, "")
	if err != nil {
		return err
	}
	return expectProblemResponse(response, notFoundProblem())
}

func (s *suite) openapiConformance(ctx context.Context) error {
	response, err := s.jsonRequest(ctx, http.MethodGet, "/tasks", "")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := expectStatusAndType(response, http.StatusOK, "application/json"); err != nil {
		return err
	}
	if _, err := s.decodeTaskList(response.Body); err != nil {
		return fmt.Errorf("list response shape: %w", err)
	}
	response, err = s.jsonRequest(ctx, http.MethodPost, "/tasks", `{"title":"ok","extra":true}`)
	if err != nil {
		return err
	}
	return expectProblemResponse(response, validationProblem())
}

func (s *suite) problemDetails(ctx context.Context) error {
	checks := []struct {
		path string
		want problem
	}{
		{"/tasks/not-a-uuid", validationProblem()},
		{"/tasks/ffffffff-ffff-4fff-8fff-ffffffffffff", notFoundProblem()},
	}
	for _, check := range checks {
		response, err := s.jsonRequest(ctx, http.MethodGet, check.path, "")
		if err != nil {
			return err
		}
		if err := expectProblemResponse(response, check.want); err != nil {
			return err
		}
	}
	return nil
}

func (s *suite) databaseConsistency(ctx context.Context) error {
	response, err := s.jsonRequest(ctx, http.MethodPost, "/tasks", `{"title":"  persisted  "}`)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := expectStatusAndType(response, http.StatusCreated, "application/json"); err != nil {
		return err
	}
	created, err := s.decodeTask(response.Body)
	if err != nil {
		return err
	}
	var title string
	var createdAt time.Time
	if err := s.db.QueryRow(ctx, "SELECT title, created_at FROM tasks WHERE id = $1", created.ID).Scan(&title, &createdAt); err != nil {
		return fmt.Errorf("query created row: %w", err)
	}
	if title != created.Title || createdAt.UTC().Format("2006-01-02T15:04:05.000000Z") != created.CreatedAt {
		return fmt.Errorf("database row does not match response: title=%q createdAt=%s", title, createdAt.UTC())
	}
	deleteResponse, err := s.jsonRequest(ctx, http.MethodDelete, "/tasks/"+created.ID, "")
	if err != nil {
		return err
	}
	deleteResponse.Body.Close()
	if deleteResponse.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete status = %d, want 204", deleteResponse.StatusCode)
	}
	var count int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM tasks WHERE id = $1", created.ID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return errors.New("deleted task remains in database")
	}
	return nil
}

func (s *suite) jsonRequest(ctx context.Context, method, path, body string) (*http.Response, error) {
	return s.jsonRequestWithHeaders(ctx, method, path, body, nil)
}

func (s *suite) jsonRequestWithHeaders(ctx context.Context, method, path, body string, headers http.Header) (*http.Response, error) {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	return s.client.Do(request)
}

func expectStatusAndType(response *http.Response, status int, mediaType string) error {
	if response.StatusCode != status {
		return fmt.Errorf("status = %d, want %d; body=%s", response.StatusCode, status, readEvidence(response.Body, responseBodyEvidenceLimit))
	}
	actualType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || actualType != mediaType {
		return fmt.Errorf("Content-Type = %q, want %s", response.Header.Get("Content-Type"), mediaType)
	}
	return nil
}

func expectProblemResponse(response *http.Response, want problem) error {
	defer response.Body.Close()
	if err := expectStatusAndType(response, want.Status, "application/problem+json"); err != nil {
		return err
	}
	var got problem
	if err := decodeExact(response.Body, &got); err != nil {
		return fmt.Errorf("Problem Details shape: %w", err)
	}
	if got != want {
		return fmt.Errorf("Problem Details = %+v, want %+v", got, want)
	}
	return nil
}

func decodeTask(reader io.Reader) (task, error) {
	return decodeTaskFor(reader, TaskBaseline)
}

func (s *suite) decodeTask(reader io.Reader) (task, error) {
	return decodeTaskFor(reader, s.task)
}

func decodeTaskFor(reader io.Reader, selectedTask string) (task, error) {
	var value task
	if err := decodeExact(reader, &value); err != nil {
		return task{}, fmt.Errorf("task response shape: %w", err)
	}
	if err := validateTaskValue(value, selectedTask); err != nil {
		return task{}, err
	}
	return value, nil
}

func validateTaskValue(value task, selectedTask string) error {
	if value.ID == "" || value.Title == "" || value.CreatedAt == "" {
		return errors.New("task response has an empty required field")
	}
	switch selectedTask {
	case TaskNullable:
		if value.DueAt == nil {
			return errors.New("task response is missing dueAt")
		}
		if value.Version != nil {
			return errors.New("task response unexpectedly contains version")
		}
	case TaskLocking:
		if value.Version == nil || *value.Version < 1 {
			return errors.New("task response has invalid or missing version")
		}
		if value.DueAt != nil {
			return errors.New("task response unexpectedly contains dueAt")
		}
	default:
		if value.DueAt != nil || value.Version != nil {
			return errors.New("baseline task response contains feature fields")
		}
	}
	return nil
}

func decodeTaskList(reader io.Reader) (taskList, error) {
	return decodeTaskListFor(reader, TaskBaseline)
}

func (s *suite) decodeTaskList(reader io.Reader) (taskList, error) {
	return decodeTaskListFor(reader, s.task)
}

func decodeTaskListFor(reader io.Reader, selectedTask string) (taskList, error) {
	var value taskList
	if err := decodeExact(reader, &value); err != nil {
		return taskList{}, err
	}
	if value.Items == nil {
		return taskList{}, errors.New("items is missing or null")
	}
	for _, item := range value.Items {
		if err := validateTaskValue(item, selectedTask); err != nil {
			return taskList{}, fmt.Errorf("invalid list item: %w", err)
		}
	}
	if selectedTask != TaskPagination && value.NextCursor != nil {
		return taskList{}, errors.New("list response unexpectedly contains nextCursor")
	}
	return value, nil
}

func decodeExact(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

var uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var timestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$`)

func isUUIDv4(value string) bool {
	return uuidV4Pattern.MatchString(value)
}

func isCanonicalTimestamp(value string) bool {
	if !timestampPattern.MatchString(value) {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func validationProblem() problem {
	return problem{Type: "urn:problem:validation", Title: "Validation failed", Status: 400, Detail: "The request is invalid."}
}

func notFoundProblem() problem {
	return problem{Type: "urn:problem:not-found", Title: "Task not found", Status: 404, Detail: "The requested task does not exist."}
}

func preconditionRequiredProblem() problem {
	return problem{Type: "urn:problem:precondition-required", Title: "Precondition required", Status: 428, Detail: "A valid If-Match header is required."}
}

func preconditionFailedProblem() problem {
	return problem{Type: "urn:problem:precondition-failed", Title: "Precondition failed", Status: 412, Detail: "The supplied task version is stale."}
}

func taskIDs(tasks []task) []string {
	ids := make([]string, len(tasks))
	for index, item := range tasks {
		ids[index] = item.ID
	}
	return ids
}

func checkTaskIDs(tasks []task, expectedIDs []string) error {
	if len(tasks) != len(expectedIDs) {
		return fmt.Errorf("got %d items, want %d", len(tasks), len(expectedIDs))
	}
	for index, id := range expectedIDs {
		if tasks[index].ID != id {
			return fmt.Errorf("ids = %v, want %v", taskIDs(tasks), expectedIDs)
		}
	}
	return nil
}

// CheckOrderedList exposes the production list checker to known-broken self-tests.
func CheckOrderedList(body []byte, expectedIDs []string) error {
	list, err := decodeTaskList(bytes.NewReader(body))
	if err != nil {
		return err
	}
	return checkTaskIDs(list.Items, expectedIDs)
}

// CheckProblem exposes the production Problem Details checker to known-broken self-tests.
func CheckProblem(status int, contentType string, body []byte, wantStatus int) error {
	response := &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}
	response.Header.Set("Content-Type", contentType)
	want := validationProblem()
	want.Status = wantStatus
	return expectProblemResponse(response, want)
}
