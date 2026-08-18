package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func validateSchema(schemaPath string, document []byte, omitDocumentSchema bool) error {
	schemaData, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read schema %s: %w", schemaPath, err)
	}
	schemaDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaData))
	if err != nil {
		return fmt.Errorf("parse schema %s: %w", schemaPath, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	const resource = "https://freezecheck.invalid/schema.json"
	if err := compiler.AddResource(resource, schemaDoc); err != nil {
		return fmt.Errorf("register schema: %w", err)
	}
	compiled, err := compiler.Compile(resource)
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	var instance any
	if err := json.Unmarshal(document, &instance); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	if omitDocumentSchema {
		if object, ok := instance.(map[string]any); ok {
			delete(object, "$schema")
		}
	}
	if err := compiled.Validate(instance); err != nil {
		return fmt.Errorf("schema validation: %w", err)
	}
	return nil
}

func readJSON(path string, out any) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(out); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode %s: trailing JSON value", path)
		}
		return nil, fmt.Errorf("decode %s: trailing data: %w", path, err)
	}
	return data, nil
}

func diagnostics(errors []string) error {
	if len(errors) == 0 {
		return nil
	}
	sortStrings(errors)
	var b bytes.Buffer
	fmt.Fprintf(&b, "%d validation error(s):", len(errors))
	for _, message := range errors {
		fmt.Fprintf(&b, "\n- %s", message)
	}
	return fmt.Errorf("%s", b.String())
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func isTODO(value string) bool { return len(value) >= 4 && value[:4] == "TODO" }
