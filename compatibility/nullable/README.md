# Nullable Generation Compatibility Probe

Status: **GO**. The selected generator and configuration preserve all three request states at the generated strict-server boundary.

## Result

The generated `PatchTaskRequest.DueAt` field is exactly `nullable.Nullable[time.Time]`. `PatchTaskRequestObject.Body` carries that generated request type into the strict handler interface. Automated decoding tests prove these distinct states:

```text
field omitted                  -> IsSpecified() false, IsNull() false
field present with null        -> IsSpecified() true,  IsNull() true
field present with a timestamp -> IsSpecified() true,  IsNull() false, Get() returns the timestamp
```

This is a go decision for the nullable PATCH task. No task or methodology change is required.

## Generator

- `oapi-codegen`: `v2.8.0`
- Go module checksum: `h1:s4hxMxuqtR8jPzXkBTtFwY/SBuj3gEAYikmbBSdtLMM=`
- `oapi-codegen/nullable`: `v1.2.0`
- `oapi-codegen/runtime`: `v1.7.0`
- `chi/v5`: `v5.3.1`
- Generated file SHA-256: `da8ba04a128e1b4d59641b2da2863ed29756d7f5940f7a657d9b00f3aaa6fe25`

The generator release and nullable behavior are documented by the official sources:

- <https://github.com/oapi-codegen/oapi-codegen/releases/tag/v2.8.0>
- <https://github.com/oapi-codegen/oapi-codegen/blob/v2.8.0/README.md#generating-nullable-types>

## Exact Configuration

`generator.yaml` is the configuration used to create `nullable.gen.go`:

```yaml
package: nullableprobe
generate:
  models: true
  chi-server: true
  strict-server: true
output-options:
  nullable-type: true
output: nullable.gen.go
```

The critical opt-in is `output-options.nullable-type: true`. The OpenAPI property is optional and declares `type: string`, `format: date-time`, and `nullable: true`.

## Reproduction

From this directory:

```sh
go generate ./...
go test ./...
```

`go generate` invokes the `v2.8.0` tool dependency pinned by `go.mod`. `nullable_test.go` contains compile-time assertions for both the generated model and strict request object, plus JSON decoding tests for omitted, explicit null, and timestamp values.

## Artifacts

- minimal OpenAPI input;
- exact generator config;
- generated Go type;
- compile-time and JSON decode tests for all three states;
- exact generator version and checksum;
- this go/no-go result.

If the probe fails, measured runs must not start. Change the generator configuration or replace the nullable task, then rerun all affected pilots.
