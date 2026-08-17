package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyPatch(t *testing.T) {
	patch := `diff --git a/api/openapi.yaml b/api/openapi.yaml
--- a/api/openapi.yaml
+++ b/api/openapi.yaml
@@ -1,2 +1,3 @@
 line
+added contract
-removed contract
diff --git a/internal/service/handler.go b/internal/service/handler.go
--- a/internal/service/handler.go
+++ b/internal/service/handler.go
@@ -1 +1,2 @@
 old
+added handwritten
diff --git a/internal/repository/generated/db.go b/internal/repository/generated/db.go
--- a/internal/repository/generated/db.go
+++ b/internal/repository/generated/db.go
@@ -1 +1,2 @@
 gen
+added generated
diff --git a/db/queries/old.sql b/db/queries/old.sql
--- a/db/queries/old.sql
+++ /dev/null
@@ -1 +0,0 @@
-removed contract deleted file
`
	dir := t.TempDir()
	path := filepath.Join(dir, "final.patch")
	if err := os.WriteFile(path, []byte(patch), 0o644); err != nil {
		t.Fatal(err)
	}

	diff, err := classifyPatch(path, "codegen")
	if err != nil {
		t.Fatal(err)
	}
	if diff.ContractLines != 3 {
		t.Fatalf("contract lines = %d, want 3", diff.ContractLines)
	}
	if diff.HandwrittenLines != 1 {
		t.Fatalf("handwritten lines = %d, want 1", diff.HandwrittenLines)
	}
	if diff.GeneratedLines != 1 {
		t.Fatalf("generated lines = %d, want 1", diff.GeneratedLines)
	}

	diff, err = classifyPatch(path, "direct")
	if err != nil {
		t.Fatal(err)
	}
	if diff.GeneratedLines != 0 {
		t.Fatal("direct treatment never classifies generated lines")
	}
	if diff.HandwrittenLines != 2 {
		t.Fatalf("direct handwritten lines = %d, want 2", diff.HandwrittenLines)
	}
}

func TestClassifyPatchEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "final.patch")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	diff, err := classifyPatch(path, "direct")
	if err != nil {
		t.Fatal(err)
	}
	if diff.ContractLines != 0 || diff.HandwrittenLines != 0 || diff.GeneratedLines != 0 {
		t.Fatalf("empty patch: %+v", diff)
	}
}
