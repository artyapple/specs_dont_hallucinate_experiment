package nullableprobe

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/oapi-codegen/nullable"
)

func generatedDueAt(request PatchTaskRequest) nullable.Nullable[time.Time] {
	return request.DueAt
}

var _ func(PatchTaskRequest) nullable.Nullable[time.Time] = generatedDueAt

func generatedStrictDueAt(request PatchTaskRequestObject) nullable.Nullable[time.Time] {
	return request.Body.DueAt
}

var _ func(PatchTaskRequestObject) nullable.Nullable[time.Time] = generatedStrictDueAt

func TestPatchTaskRequestDistinguishesDueAtStates(t *testing.T) {
	wantTime := time.Date(2026, time.August, 16, 17, 18, 19, 123456000, time.UTC)
	tests := []struct {
		name          string
		body          string
		wantSpecified bool
		wantNull      bool
		wantValue     *time.Time
	}{
		{
			name: "omitted",
			body: `{}`,
		},
		{
			name:          "explicit null",
			body:          `{"dueAt":null}`,
			wantSpecified: true,
			wantNull:      true,
		},
		{
			name:          "timestamp value",
			body:          `{"dueAt":"2026-08-16T17:18:19.123456Z"}`,
			wantSpecified: true,
			wantValue:     &wantTime,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request PatchTaskRequest
			if err := json.Unmarshal([]byte(tt.body), &request); err != nil {
				t.Fatalf("decode request: %v", err)
			}

			if got := request.DueAt.IsSpecified(); got != tt.wantSpecified {
				t.Fatalf("IsSpecified() = %v, want %v", got, tt.wantSpecified)
			}
			if got := request.DueAt.IsNull(); got != tt.wantNull {
				t.Fatalf("IsNull() = %v, want %v", got, tt.wantNull)
			}

			got, err := request.DueAt.Get()
			if tt.wantValue == nil {
				if err == nil {
					t.Fatalf("Get() unexpectedly returned value %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Get(): %v", err)
			}
			if !got.Equal(*tt.wantValue) {
				t.Fatalf("Get() = %s, want %s", got, *tt.wantValue)
			}
		})
	}
}
