package main

import "time"

// Run-result identity and lifecycle statuses. The values mirror
// schemas/run-result.schema.json; keep the constants aligned with the schema.
const (
	statusSubmitted   = "submitted"
	statusTimedOut    = "timed-out"
	statusInfraFail   = "infrastructure-failure"
	statusHarnessFail = "harness-failure"
)

// Infrastructure failure categories follow config/infrastructure-failure-policy.md.
// Exclusion eligibility remains a reviewed decision; the assembler never sets it.
const (
	infraCategoryProviderOutage   = "model-provider-outage"
	infraCategoryHarnessCrash     = "harness-process-crash"
	infraCategoryRuntimeFailure   = "host-container-runtime-failure"
	infraCategoryEvaluatorFailure = "evaluator-infrastructure-failure"
	infraCategoryArtifactStorage  = "artifact-storage-failure"
)

// RunMetadata is the harness-owned run-dir/metadata.json contract consumed by
// the assembler. The run driver writes it before the agent session starts and
// finalizes status and finishedAt when the session ends. All fields are draft
// until the global freeze.
type RunMetadata struct {
	RunID              string              `json:"runId"`
	CellID             string              `json:"cellId"`
	RepeatIndex        int                 `json:"repeatIndex"`
	Phase              string              `json:"phase"`
	Status             string              `json:"status"`
	StartedAt          time.Time           `json:"startedAt"`
	FinishedAt         time.Time           `json:"finishedAt"`
	Workspace          string              `json:"workspace"`
	ProtocolViolations []ProtocolViolation `json:"protocolViolations"`
	Infrastructure     *InfrastructureNote `json:"infrastructure"`
	ReplacesRunID      *string             `json:"replacesRunId"`
	Orchestration      *Orchestration      `json:"orchestration,omitempty"`
}

// Orchestration records production-only provenance without changing the
// published run-result schema. Synthetic drivers omit it.
type Orchestration struct {
	SchedulePath      string            `json:"schedulePath"`
	ScheduleOrdinal   int               `json:"scheduleOrdinal"`
	ScheduleRunID     string            `json:"scheduleRunId"`
	Model             string            `json:"model"`
	AgentVersion      string            `json:"agentVersion"`
	ResolvedSources   map[string]string `json:"resolvedSources"`
	Images            map[string]string `json:"images"`
	TimeoutSeconds    int               `json:"timeoutSeconds"`
	ResourceLabels    map[string]string `json:"resourceLabels"`
	CandidateExitCode int               `json:"candidateExitCode"`
	CandidateSignal   string            `json:"candidateSignal,omitempty"`
}

// InfrastructureNote carries the driver's failure classification for runs whose
// input status is infrastructure-failure or harness-failure.
type InfrastructureNote struct {
	Category string `json:"category"`
	Evidence string `json:"evidence"`
}

type ProtocolViolation struct {
	Category                   string `json:"category"`
	Timestamp                  string `json:"timestamp"`
	Evidence                   string `json:"evidence"`
	IsolationForcedTermination bool   `json:"isolationForcedTermination"`
}

// Cell is one experiment matrix cell resolved from config/experiment.json.
type Cell struct {
	ID        string `json:"id"`
	Stage     string `json:"stage"`
	Task      string `json:"task"`
	Mode      string `json:"mode"`
	Treatment string `json:"treatment"`
}

type experimentConfig struct {
	Cells []Cell `json:"cells"`
}

// Evaluator output mirrors evaluator/internal/evaluator Result. The standalone
// evaluator JSON is preserved as evaluation.json; these types are its reader.
type evaluatorResult struct {
	CompleteSuccess bool            `json:"completeSuccess"`
	Setup           evaluatorSetup  `json:"setup"`
	BehaviorCases   []evaluatorCase `json:"behaviorCases"`
}

type evaluatorSetup struct {
	Postgres   bool   `json:"postgres"`
	Build      bool   `json:"build"`
	Migrations bool   `json:"migrations"`
	Ready      bool   `json:"serviceReady"`
	Evidence   string `json:"evidence"`
}

type evaluatorCase struct {
	ID         string `json:"id"`
	Applicable bool   `json:"applicable"`
	Passed     *bool  `json:"passed"`
	Evidence   string `json:"evidence"`
}

type caseManifest struct {
	Cases []struct {
		ID   string `json:"id"`
		Task string `json:"task"`
	} `json:"cases"`
}

// RunResult mirrors schemas/run-result.schema.json exactly. Nullable schema
// fields use pointers without omitempty so they encode explicit JSON null.
type RunResult struct {
	SchemaVersion      int                 `json:"schemaVersion"`
	RunID              string              `json:"runId"`
	CellID             string              `json:"cellId"`
	RepeatIndex        int                 `json:"repeatIndex"`
	Phase              string              `json:"phase"`
	Stage              string              `json:"stage"`
	Task               string              `json:"task"`
	Mode               string              `json:"mode"`
	Treatment          string              `json:"treatment"`
	Status             string              `json:"status"`
	ProtocolViolations []ProtocolViolation `json:"protocolViolations"`
	Infrastructure     Infrastructure      `json:"infrastructure"`
	Timing             Timing              `json:"timing"`
	Usage              Usage               `json:"usage"`
	Process            Process             `json:"process"`
	Artifacts          Artifacts           `json:"artifacts"`
	Evaluation         *Evaluation         `json:"evaluation"`
}

type Infrastructure struct {
	IsFailure         bool    `json:"isFailure"`
	Category          *string `json:"category"`
	Evidence          *string `json:"evidence"`
	ExclusionEligible bool    `json:"exclusionEligible"`
	Excluded          bool    `json:"excluded"`
	ReplacementRunID  *string `json:"replacementRunId"`
	ReplacesRunID     *string `json:"replacesRunId"`
}

type Timing struct {
	StartedAt                               time.Time `json:"startedAt"`
	FinishedAt                              time.Time `json:"finishedAt"`
	WallClockMilliseconds                   int64     `json:"wallClockMilliseconds"`
	FirstSuccessfulBuildMilliseconds        *int64    `json:"firstSuccessfulBuildMilliseconds"`
	FirstVisibleBehaviorSuccessMilliseconds *int64    `json:"firstVisibleBehaviorSuccessMilliseconds"`
}

type Usage struct {
	InputTokens  *int64 `json:"inputTokens"`
	OutputTokens *int64 `json:"outputTokens"`
	Turns        int64  `json:"turns"`
	ToolCalls    int64  `json:"toolCalls"`
}

type Process struct {
	RepairIterations int64           `json:"repairIterations"`
	FilesTouched     int64           `json:"filesTouched"`
	Diff             DiffLines       `json:"diff"`
	CompilerEvents   []CompilerEvent `json:"compilerEvents"`
}

type DiffLines struct {
	ContractLines    int64 `json:"contractLines"`
	HandwrittenLines int64 `json:"handwrittenLines"`
	GeneratedLines   int64 `json:"generatedLines"`
}

type CompilerEvent struct {
	CommandIndex                  int64    `json:"commandIndex"`
	Category                      string   `json:"category"`
	Files                         []string `json:"files"`
	Locations                     []string `json:"locations"`
	PointsToHandwrittenAdaptation *bool    `json:"pointsToHandwrittenAdaptation"`
	FollowedByRelevantRepair      *bool    `json:"followedByRelevantRepair"`
}

type Artifacts struct {
	Metadata          string `json:"metadata"`
	Transcript        string `json:"transcript"`
	FinalPatch        string `json:"finalPatch"`
	Commands          string `json:"commands"`
	Evaluation        string `json:"evaluation"`
	WorkspaceManifest string `json:"workspaceManifest"`
}

type Evaluation struct {
	CompleteSuccess  bool           `json:"completeSuccess"`
	CommonGates      CommonGates    `json:"commonGates"`
	BehaviorCases    []BehaviorCase `json:"behaviorCases"`
	CodegenHealth    *CodegenHealth `json:"codegenHealth"`
	CandidateTests   CandidateTests `json:"candidateTests"`
	ResidualFailures []string       `json:"residualFailures"`
}

type CommonGates struct {
	FormalInputs        bool `json:"formal-inputs"`
	Build               bool `json:"build"`
	Migrations          bool `json:"migrations"`
	ServiceStart        bool `json:"service-start"`
	BaselineBehavior    bool `json:"baseline-behavior"`
	TaskBehavior        bool `json:"task-behavior"`
	Regressions         bool `json:"regressions"`
	APIConformance      bool `json:"api-conformance"`
	DatabaseConsistency bool `json:"database-consistency"`
}

type BehaviorCase struct {
	ID         string  `json:"id"`
	Applicable bool    `json:"applicable"`
	Passed     *bool   `json:"passed"`
	Evidence   *string `json:"evidence"`
}

type CodegenHealth struct {
	GenerationSucceeded bool `json:"generationSucceeded"`
	Canonical           bool `json:"canonical"`
	Idempotent          bool `json:"idempotent"`
	ManualEditDetected  bool `json:"manualEditDetected"`
}

type CandidateTests struct {
	Present   bool  `json:"present"`
	TestFiles int64 `json:"testFiles"`
}
