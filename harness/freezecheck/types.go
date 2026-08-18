package main

type cell struct {
	ID        string `json:"id"`
	Stage     string `json:"stage"`
	Task      string `json:"task"`
	Mode      string `json:"mode"`
	Treatment string `json:"treatment"`
}

type experimentConfig struct {
	Status string `json:"status"`
	Model  struct {
		Provider string `json:"provider"`
	} `json:"model"`
	Agent struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"agent"`
	FrozenInputs struct {
		ConfigRevision       string            `json:"configRevision"`
		DesignRevision       string            `json:"designRevision"`
		EvaluatorRevision    string            `json:"evaluatorRevision"`
		ResultSchemaRevision string            `json:"resultSchemaRevision"`
		EnvironmentImages    map[string]string `json:"environmentImages"`
		ScheduleManifest     string            `json:"scheduleManifest"`
		Fixtures             map[string]string `json:"fixtures"`
		Treatments           map[string]string `json:"treatments"`
		Tasks                map[string]string `json:"tasks"`
	} `json:"frozenInputs"`
	Execution struct {
		NetworkPolicyEnforcementStatus string `json:"networkPolicyEnforcementStatus"`
		Schedule                       string `json:"schedule"`
		ScheduleSeed                   string `json:"scheduleSeed"`
	} `json:"execution"`
	Cells []cell `json:"cells"`
}

type schedule struct {
	Schema         string        `json:"$schema"`
	SchemaVersion  int           `json:"schemaVersion"`
	Status         string        `json:"status"`
	Strategy       string        `json:"strategy"`
	Algorithm      string        `json:"algorithm"`
	Seed           string        `json:"seed"`
	GeneratedAt    *string       `json:"generatedAt"`
	ConfigRevision string        `json:"configRevision"`
	Runs           []scheduleRun `json:"runs"`
}

type scheduleRun struct {
	Ordinal     int    `json:"ordinal"`
	RunID       string `json:"runId"`
	CellID      string `json:"cellId"`
	RepeatIndex int    `json:"repeatIndex"`
}

type runResult struct {
	RunID          string `json:"runId"`
	CellID         string `json:"cellId"`
	RepeatIndex    int    `json:"repeatIndex"`
	Phase          string `json:"phase"`
	Stage          string `json:"stage"`
	Task           string `json:"task"`
	Mode           string `json:"mode"`
	Treatment      string `json:"treatment"`
	Status         string `json:"status"`
	Infrastructure struct {
		IsFailure         bool    `json:"isFailure"`
		Category          *string `json:"category"`
		Evidence          *string `json:"evidence"`
		ExclusionEligible bool    `json:"exclusionEligible"`
		Excluded          bool    `json:"excluded"`
		ReplacementRunID  *string `json:"replacementRunId"`
		ReplacesRunID     *string `json:"replacesRunId"`
	} `json:"infrastructure"`
	Artifacts struct {
		Metadata          string `json:"metadata"`
		Transcript        string `json:"transcript"`
		FinalPatch        string `json:"finalPatch"`
		Commands          string `json:"commands"`
		Evaluation        string `json:"evaluation"`
		WorkspaceManifest string `json:"workspaceManifest"`
	} `json:"artifacts"`
	Evaluation *evaluation `json:"evaluation"`
}

type evaluation struct {
	CompleteSuccess bool            `json:"completeSuccess"`
	CommonGates     map[string]bool `json:"commonGates"`
	BehaviorCases   []behaviorCase  `json:"behaviorCases"`
}

type behaviorCase struct {
	ID         string  `json:"id"`
	Applicable bool    `json:"applicable"`
	Passed     *bool   `json:"passed"`
	Evidence   *string `json:"evidence"`
}

type caseManifest struct {
	Cases []struct {
		ID       string `json:"id"`
		Task     string `json:"task"`
		Required bool   `json:"required"`
	} `json:"cases"`
}
