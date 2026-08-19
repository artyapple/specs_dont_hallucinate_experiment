package evaluator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestKnownBrokenCandidatesAreRejected(t *testing.T) {
	validTask := func(id string) map[string]any {
		return map[string]any{
			"id":        id,
			"title":     "task",
			"createdAt": "2000-01-01T00:00:00.000000Z",
		}
	}

	tests := []struct {
		name   string
		caseID string
		check  func() error
	}{
		{
			name:   "reverse list order",
			caseID: "baseline.list-ordered",
			check: func() error {
				body, _ := json.Marshal(map[string]any{"items": []any{
					validTask("00000000-0000-4000-8000-000000000002"),
					validTask("00000000-0000-4000-8000-000000000001"),
				}})
				return CheckOrderedList(body, []string{
					"00000000-0000-4000-8000-000000000001",
					"00000000-0000-4000-8000-000000000002",
				})
			},
		},
		{
			name:   "extra response field",
			caseID: "contract.openapi-conformance",
			check: func() error {
				item := validTask("00000000-0000-4000-8000-000000000001")
				item["unexpected"] = true
				body, _ := json.Marshal(map[string]any{"items": []any{item}})
				return CheckOrderedList(body, []string{"00000000-0000-4000-8000-000000000001"})
			},
		},
		{
			name:   "wrong problem content type",
			caseID: "contract.problem-details",
			check: func() error {
				body := []byte(`{"type":"urn:problem:validation","title":"Validation failed","status":400,"detail":"The request is invalid."}`)
				return CheckProblem(400, "application/json", body, 400)
			},
		},
		{
			name:   "wrong problem detail",
			caseID: "contract.problem-details",
			check: func() error {
				body := []byte(`{"type":"urn:problem:validation","title":"Validation failed","status":400,"detail":"internal database error"}`)
				return CheckProblem(400, "application/problem+json", body, 400)
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.caseID+"/"+testCase.name, func(t *testing.T) {
			assertKnownBroken(t, testCase.caseID, testCase.check)
		})
	}
}

func assertKnownBroken(t *testing.T, expectedCaseID string, check func() error) {
	t.Helper()
	if expectedCaseID == "" {
		t.Fatal("known-broken candidate has no expected failing case ID")
	}
	if err := check(); err == nil {
		t.Fatalf("known-broken candidate passed %s", expectedCaseID)
	}
}

func TestKnownBrokenNonPersistingCandidateIsRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
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
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatal(err)
	}
	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `CREATE TABLE tasks (
		id uuid PRIMARY KEY,
		title text NOT NULL,
		created_at timestamp(6) with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatal(err)
	}

	const id = "00000000-0000-4000-8000-000000000001"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		fmt.Fprintf(writer, `{"id":%q,"title":"persisted","createdAt":"2000-01-01T00:00:00.000000Z"}`, id)
	}))
	defer server.Close()

	broken := &suite{baseURL: server.URL, client: server.Client(), db: pool}
	assertKnownBroken(t, "contract.database-consistency", func() error {
		return broken.databaseConsistency(ctx)
	})
}

func TestKnownBrokenNullableOmittedValueIsRejected(t *testing.T) {
	dueAt := ""
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case http.MethodPost:
			writeNullableTask(writer, "original", "")
		case http.MethodPatch:
			var body map[string]json.RawMessage
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Error(err)
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			title := "original"
			if raw, ok := body["dueAt"]; ok {
				if err := json.Unmarshal(raw, &dueAt); err != nil {
					t.Error(err)
				}
			} else {
				// Known bug: an omitted dueAt is treated as null and clears the value.
				dueAt = ""
				_ = json.Unmarshal(body["title"], &title)
			}
			writeNullableTask(writer, title, dueAt)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	broken := &suite{baseURL: server.URL, client: server.Client(), task: TaskNullable}
	assertKnownBroken(t, "nullable.omitted-preserves", func() error {
		return broken.nullableOmittedPreserves(context.Background())
	})
}

func TestKnownBrokenNullableNullAndValueConfusionIsRejected(t *testing.T) {
	tests := []struct {
		name           string
		expectedCaseID string
		patchDueAt     func(current string, raw json.RawMessage) string
		check          func(*suite) error
	}{
		{
			name:           "null treated as omitted",
			expectedCaseID: "nullable.null-clears",
			patchDueAt: func(current string, raw json.RawMessage) string {
				if string(raw) == "null" {
					return current
				}
				var value string
				_ = json.Unmarshal(raw, &value)
				return value
			},
			check: func(broken *suite) error {
				return broken.nullableNullClears(context.Background())
			},
		},
		{
			name:           "value treated as null",
			expectedCaseID: "nullable.value-sets",
			patchDueAt: func(_ string, _ json.RawMessage) string {
				return ""
			},
			check: func(broken *suite) error {
				return broken.nullableValueSets(context.Background())
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.expectedCaseID+"/"+testCase.name, func(t *testing.T) {
			dueAt := ""
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				switch request.Method {
				case http.MethodPost:
					writeNullableTask(writer, "original", "")
				case http.MethodPatch:
					var body map[string]json.RawMessage
					if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
						t.Error(err)
						writer.WriteHeader(http.StatusBadRequest)
						return
					}
					if raw, ok := body["dueAt"]; ok {
						dueAt = testCase.patchDueAt(dueAt, raw)
					}
					writeNullableTask(writer, "original", dueAt)
				default:
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer server.Close()

			broken := &suite{baseURL: server.URL, client: server.Client(), task: TaskNullable}
			assertKnownBroken(t, testCase.expectedCaseID, func() error { return testCase.check(broken) })
		})
	}
}

func TestKnownBrokenLockingETagBehaviorIsRejected(t *testing.T) {
	t.Run("locking.malformed-if-match/accepts malformed tags", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			fmt.Fprint(writer, `{"id":"00000000-0000-4000-8000-000000000001","title":"changed","createdAt":"2000-01-01T00:00:00.000000Z","version":2}`)
		}))
		defer server.Close()

		broken := &suite{baseURL: server.URL, client: server.Client(), task: TaskLocking}
		assertKnownBroken(t, "locking.malformed-if-match", func() error {
			return broken.lockingMalformedIfMatch(context.Background())
		})
	})

	t.Run("locking.stale-if-match/accepts stale tag", func(t *testing.T) {
		version := 1
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			switch request.Method {
			case http.MethodPost:
				writer.Header().Set("ETag", `"1"`)
				fmt.Fprint(writer, `{"id":"00000000-0000-4000-8000-000000000001","title":"original","createdAt":"2000-01-01T00:00:00.000000Z","version":1}`)
			case http.MethodPut:
				version++
				writer.Header().Set("ETag", fmt.Sprintf(`"%d"`, version))
				fmt.Fprintf(writer, `{"id":"00000000-0000-4000-8000-000000000001","title":"changed","createdAt":"2000-01-01T00:00:00.000000Z","version":%d}`, version)
			default:
				writer.WriteHeader(http.StatusMethodNotAllowed)
			}
		}))
		defer server.Close()

		broken := &suite{baseURL: server.URL, client: server.Client(), task: TaskLocking}
		assertKnownBroken(t, "locking.stale-if-match", func() error {
			return broken.lockingStaleIfMatch(context.Background())
		})
	})
}

func TestKnownBrokenLockingTwoWinnersIsRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case http.MethodPost:
			writer.Header().Set("ETag", `"1"`)
			fmt.Fprint(writer, `{"id":"00000000-0000-4000-8000-000000000001","title":"original","createdAt":"2000-01-01T00:00:00.000000Z","version":1}`)
		case http.MethodPut:
			var body struct {
				Title string `json:"title"`
			}
			_ = json.NewDecoder(request.Body).Decode(&body)
			writer.Header().Set("ETag", `"2"`)
			fmt.Fprintf(writer, `{"id":"00000000-0000-4000-8000-000000000001","title":%q,"createdAt":"2000-01-01T00:00:00.000000Z","version":2}`, body.Title)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	broken := &suite{baseURL: server.URL, client: server.Client(), task: TaskLocking}
	assertKnownBroken(t, "locking.concurrent-single-winner", func() error {
		return broken.lockingConcurrentSingleWinner(context.Background())
	})
}

func TestKnownBrokenPaginationDuplicateIsRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Query().Get("cursor") == "" {
			fmt.Fprint(writer, `{"items":[`+
				paginationTaskJSON("00000000-0000-4000-8000-000000000001")+`,`+
				paginationTaskJSON("00000000-0000-4000-8000-000000000002")+
				`],"nextCursor":"next"}`)
			return
		}
		fmt.Fprint(writer, `{"items":[`+
			paginationTaskJSON("00000000-0000-4000-8000-000000000002")+`,`+
			paginationTaskJSON("00000000-0000-4000-8000-000000000003")+
			`]}`)
	}))
	defer server.Close()

	broken := &suite{baseURL: server.URL, client: server.Client(), task: TaskPagination}
	got, err := broken.collectPages(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-000000000002",
		"00000000-0000-4000-8000-000000000003",
	}
	assertKnownBroken(t, "pagination.multiple-pages", func() error { return checkStringIDs(got, want) })
}

func TestKnownBrokenPaginationOrderingTiesAndGapsAreRejected(t *testing.T) {
	tests := []struct {
		name           string
		expectedCaseID string
		pages          [][]string
		want           []string
	}{
		{
			name:           "unstable page order",
			expectedCaseID: "pagination.multiple-pages",
			pages:          [][]string{{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002"}, {"00000000-0000-4000-8000-000000000004", "00000000-0000-4000-8000-000000000003"}},
			want:           []string{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002", "00000000-0000-4000-8000-000000000003", "00000000-0000-4000-8000-000000000004"},
		},
		{
			name:           "gap between pages",
			expectedCaseID: "pagination.multiple-pages",
			pages:          [][]string{{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002"}, {"00000000-0000-4000-8000-000000000004"}},
			want:           []string{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002", "00000000-0000-4000-8000-000000000003", "00000000-0000-4000-8000-000000000004"},
		},
		{
			name:           "timestamp-only cursor skips tied row",
			expectedCaseID: "pagination.timestamp-tie",
			pages:          [][]string{{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002"}, {"00000000-0000-4000-8000-000000000004"}},
			want:           []string{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002", "00000000-0000-4000-8000-000000000003", "00000000-0000-4000-8000-000000000004"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.expectedCaseID+"/"+testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				pageIndex := 0
				if request.URL.Query().Get("cursor") != "" {
					pageIndex = 1
				}
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprint(writer, `{"items":[`)
				for index, id := range testCase.pages[pageIndex] {
					if index > 0 {
						fmt.Fprint(writer, ",")
					}
					fmt.Fprint(writer, paginationTaskJSON(id))
				}
				if pageIndex == 0 {
					fmt.Fprint(writer, `],"nextCursor":"next"}`)
				} else {
					fmt.Fprint(writer, `]}`)
				}
			}))
			defer server.Close()

			broken := &suite{baseURL: server.URL, client: server.Client(), task: TaskPagination}
			got, err := broken.collectPages(context.Background(), 2)
			if err != nil {
				t.Fatal(err)
			}
			assertKnownBroken(t, testCase.expectedCaseID, func() error {
				return checkStringIDs(got, testCase.want)
			})
		})
	}
}

func writeNullableTask(writer http.ResponseWriter, title, dueAt string) {
	if dueAt == "" {
		fmt.Fprintf(writer, `{"id":"00000000-0000-4000-8000-000000000001","title":%q,"createdAt":"2000-01-01T00:00:00.000000Z","dueAt":null}`, title)
		return
	}
	fmt.Fprintf(writer, `{"id":"00000000-0000-4000-8000-000000000001","title":%q,"createdAt":"2000-01-01T00:00:00.000000Z","dueAt":%q}`, title, nullableSetUTC)
}

func paginationTaskJSON(id string) string {
	return fmt.Sprintf(`{"id":%q,"title":"task","createdAt":"2000-01-01T00:00:00.000000Z"}`, id)
}

func TestCaseRegistryManifestAndSchemaAgree(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestData, err := os.ReadFile(filepath.Join(root, "case-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Cases []struct {
			ID   string `json:"id"`
			Task string `json:"task"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	manifestIDs := make(map[string]bool)
	manifestTasks := make(map[string]string)
	for _, item := range manifest.Cases {
		if manifestIDs[item.ID] {
			t.Errorf("duplicate case ID %s in manifest", item.ID)
		}
		manifestIDs[item.ID] = true
		manifestTasks[item.ID] = item.Task
	}

	schemaData, err := os.ReadFile(filepath.Join(root, "..", "schemas", "run-result.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema any
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		t.Fatal(err)
	}
	schemaIDs := collectCaseEnum(schema)

	casesMarkdown, err := os.ReadFile(filepath.Join(root, "cases.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ManifestIDs() {
		if !manifestIDs[id] {
			t.Errorf("implemented case %s is absent from case-manifest.json", id)
		}
		if !schemaIDs[id] {
			t.Errorf("implemented case %s is absent from run-result.schema.json", id)
		}
		if !strings.Contains(string(casesMarkdown), "`"+id+"`") {
			t.Errorf("implemented case %s is absent from cases.md", id)
		}
	}
	registryIDs := make(map[string]bool)
	for _, definition := range caseDefinitions() {
		if registryIDs[definition.ID] {
			t.Errorf("duplicate case ID %s in registry", definition.ID)
		}
		registryIDs[definition.ID] = true
		if manifestTasks[definition.ID] != definition.Task {
			t.Errorf("case %s task = %q in registry, want manifest task %q", definition.ID, definition.Task, manifestTasks[definition.ID])
		}
		if definition.Run == nil {
			t.Errorf("case %s has no implementation", definition.ID)
		}
	}

	allManifest := make([]string, 0, len(manifestIDs))
	allSchema := make([]string, 0, len(schemaIDs))
	for id := range manifestIDs {
		allManifest = append(allManifest, id)
	}
	for id := range schemaIDs {
		allSchema = append(allSchema, id)
	}
	sort.Strings(allManifest)
	sort.Strings(allSchema)
	if !reflect.DeepEqual(allManifest, allSchema) {
		t.Errorf("manifest and result schema case IDs differ\nmanifest: %v\nschema: %v", allManifest, allSchema)
	}
}

func TestCaseApplicabilityAndNullPassedRepresentation(t *testing.T) {
	definitions := caseDefinitions()
	expectedApplicable := map[string]int{
		TaskBaseline:   10,
		TaskNullable:   24,
		TaskLocking:    20,
		TaskPagination: 20,
	}
	for selectedTask, expected := range expectedApplicable {
		results := setupFailureCases(definitions, selectedTask, errors.New("setup failed"))
		if len(results) != len(definitions) {
			t.Fatalf("task %s emitted %d cases, want complete roster of %d", selectedTask, len(results), len(definitions))
		}
		applicable := 0
		for _, result := range results {
			if result.Applicable {
				applicable++
				if result.Passed == nil || *result.Passed {
					t.Errorf("applicable setup-failure case %s passed = %v, want false", result.ID, result.Passed)
				}
			} else if result.Passed != nil {
				t.Errorf("non-applicable case %s passed = %v, want null", result.ID, *result.Passed)
			}
		}
		if applicable != expected {
			t.Errorf("task %s applicable cases = %d, want %d", selectedTask, applicable, expected)
		}
		encoded, err := json.Marshal(results)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(encoded), `"applicable":false,"passed":null`) {
			t.Errorf("task %s output does not encode non-applicable passed as null", selectedTask)
		}
	}
}

func collectCaseEnum(value any) map[string]bool {
	result := make(map[string]bool)
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			if values, ok := typed["enum"].([]any); ok {
				for _, value := range values {
					if id, ok := value.(string); ok && strings.Contains(id, ".") {
						result[id] = true
					}
				}
			}
			for _, child := range typed {
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(value)
	return result
}
