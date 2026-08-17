package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestQueriesMatchCanonicalSQL(t *testing.T) {
	contents, err := os.ReadFile("../../../db/queries/tasks.sql")
	if err != nil {
		t.Fatalf("read canonical queries: %v", err)
	}

	sections := map[string]string{}
	var name string
	var lines []string
	flush := func() {
		if name != "" {
			sections[name] = strings.TrimSpace(strings.Join(lines, "\n"))
		}
		lines = nil
	}
	for _, line := range strings.Split(string(contents), "\n") {
		if strings.HasPrefix(line, "-- name: ") {
			flush()
			fields := strings.Fields(line)
			if len(fields) < 3 {
				t.Fatalf("invalid query marker %q", line)
			}
			name = fields[2]
			continue
		}
		lines = append(lines, line)
	}
	flush()

	want := map[string]string{
		"CreateTask": createTaskSQL,
		"GetTask":    getTaskSQL,
		"ListTasks":  listTasksSQL,
		"DeleteTask": deleteTaskSQL,
		"PatchTask":  patchTaskSQL,
	}
	if len(sections) != len(want) {
		t.Fatalf("canonical query count = %d, want %d", len(sections), len(want))
	}
	for query, sql := range want {
		canonical := sections[query]
		if query == "PatchTask" {
			for index, argument := range []string{"sqlc.arg(title_present)", "sqlc.arg(title)", "sqlc.arg(due_at_present)", "sqlc.narg(due_at)", "sqlc.arg(id)"} {
				canonical = strings.ReplaceAll(canonical, argument, "$"+string(rune('1'+index)))
			}
		}
		if canonical != sql {
			t.Errorf("%s SQL drifted\ngot:\n%s\nwant:\n%s", query, canonical, sql)
		}
	}
}
