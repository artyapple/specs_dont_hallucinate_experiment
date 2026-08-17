package evaluator

import (
	"context"
	"sort"
)

type caseDefinition struct {
	ID   string
	Task string
	Run  func(*suite, context.Context) error
}

func caseDefinitions() []caseDefinition {
	return []caseDefinition{
		{"baseline.create-valid", taskAll, (*suite).createValid},
		{"baseline.create-invalid-title", taskAll, (*suite).createInvalidTitle},
		{"baseline.get-existing", taskAll, (*suite).getExisting},
		{"baseline.get-not-found", taskAll, (*suite).getNotFound},
		{"baseline.list-ordered", taskAll, (*suite).listOrdered},
		{"baseline.delete-existing", taskAll, (*suite).deleteExisting},
		{"baseline.delete-again-not-found", taskAll, (*suite).deleteAgainNotFound},
		{"nullable.initial-null", TaskNullable, (*suite).nullableInitialNull},
		{"nullable.post-rejects-due-at", TaskNullable, (*suite).nullablePostRejectsDueAt},
		{"nullable.omitted-preserves", TaskNullable, (*suite).nullableOmittedPreserves},
		{"nullable.null-clears", TaskNullable, (*suite).nullableNullClears},
		{"nullable.value-sets", TaskNullable, (*suite).nullableValueSets},
		{"nullable.title-only", TaskNullable, (*suite).nullableTitleOnly},
		{"nullable.both-fields", TaskNullable, (*suite).nullableBothFields},
		{"nullable.empty-rejected", TaskNullable, (*suite).nullableEmptyRejected},
		{"nullable.unknown-field-rejected", TaskNullable, (*suite).nullableUnknownFieldRejected},
		{"nullable.title-null-rejected", TaskNullable, (*suite).nullableTitleNullRejected},
		{"nullable.invalid-title", TaskNullable, (*suite).nullableInvalidTitle},
		{"nullable.invalid-timestamp", TaskNullable, (*suite).nullableInvalidTimestamp},
		{"nullable.unknown-task", TaskNullable, (*suite).nullableUnknownTask},
		{"nullable.get-consistent", TaskNullable, (*suite).nullableGetConsistent},
		{"locking.initial-version", TaskLocking, (*suite).lockingInitialVersion},
		{"locking.get-etag", TaskLocking, (*suite).lockingGetETag},
		{"locking.put-success", TaskLocking, (*suite).lockingPutSuccess},
		{"locking.missing-if-match", TaskLocking, (*suite).lockingMissingIfMatch},
		{"locking.malformed-if-match", TaskLocking, (*suite).lockingMalformedIfMatch},
		{"locking.stale-if-match", TaskLocking, (*suite).lockingStaleIfMatch},
		{"locking.unknown-task", TaskLocking, (*suite).lockingUnknownTask},
		{"locking.unknown-field", TaskLocking, (*suite).lockingUnknownField},
		{"locking.invalid-title", TaskLocking, (*suite).lockingInvalidTitle},
		{"locking.concurrent-single-winner", TaskLocking, (*suite).lockingConcurrentSingleWinner},
		{"pagination.default-limit", TaskPagination, (*suite).paginationDefaultLimit},
		{"pagination.limit-bounds", TaskPagination, (*suite).paginationLimitBounds},
		{"pagination.invalid-limit", TaskPagination, (*suite).paginationInvalidLimit},
		{"pagination.malformed-cursor", TaskPagination, (*suite).paginationMalformedCursor},
		{"pagination.empty", TaskPagination, (*suite).paginationEmpty},
		{"pagination.single-page", TaskPagination, (*suite).paginationSinglePage},
		{"pagination.multiple-pages", TaskPagination, (*suite).paginationMultiplePages},
		{"pagination.timestamp-tie", TaskPagination, (*suite).paginationTimestampTie},
		{"pagination.final-page", TaskPagination, (*suite).paginationFinalPage},
		{"pagination.cursor-after-delete", TaskPagination, (*suite).paginationCursorAfterDelete},
		{"contract.openapi-conformance", taskAll, (*suite).openapiConformance},
		{"contract.problem-details", taskAll, (*suite).problemDetails},
		{"contract.database-consistency", taskAll, (*suite).databaseConsistency},
	}
}

func ManifestIDs() []string {
	definitions := caseDefinitions()
	ids := make([]string, len(definitions))
	for index, definition := range definitions {
		ids[index] = definition.ID
	}
	sort.Strings(ids)
	return ids
}
