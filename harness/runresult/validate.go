package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// validateRunResult checks the assembled document against the repository-owned
// JSON Schema. A violation is a harness defect and must stop assembly loudly.
func validateRunResult(schemaPath string, document []byte) error {
	schemaData, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	schemaDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaData))
	if err != nil {
		return fmt.Errorf("parse schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	const schemaURL = "https://example.invalid/specs-dont-hallucinate/run-result.schema.json"
	if err := compiler.AddResource(schemaURL, schemaDoc); err != nil {
		return fmt.Errorf("register schema: %w", err)
	}
	compiled, err := compiler.Compile(schemaURL)
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	var instance any
	if err := json.Unmarshal(document, &instance); err != nil {
		return fmt.Errorf("decode produced result: %w", err)
	}
	if err := compiled.Validate(instance); err != nil {
		return fmt.Errorf("schema validation: %w", err)
	}
	return nil
}
