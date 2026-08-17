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
		"UpdateTask": updateTaskSQL,
	}
	if len(sections) != len(want) {
		t.Fatalf("canonical query count = %d, want %d", len(sections), len(want))
	}
	for query, sql := range want {
		if sections[query] != sql {
			t.Errorf("%s SQL drifted\ngot:\n%s\nwant:\n%s", query, sections[query], sql)
		}
	}
}
