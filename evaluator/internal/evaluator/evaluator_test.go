package evaluator

import (
	"context"
	"encoding/json"
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
			if err := testCase.check(); err == nil {
				t.Fatalf("known-broken candidate passed %s", testCase.caseID)
			}
		})
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
	if err := broken.databaseConsistency(ctx); err == nil {
		t.Fatal("known-broken non-persisting candidate passed contract.database-consistency")
	}
}

func TestBaselineCaseIDsAgree(t *testing.T) {
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
	for _, item := range manifest.Cases {
		manifestIDs[item.ID] = true
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
