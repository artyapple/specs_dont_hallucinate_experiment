.PHONY: build-analysis-input build-evaluator build-evaluator-image build-freezecheck build-rundriver build-runresult evaluate-bases evaluate-base2-codegen evaluate-base2-direct evaluate-task-solutions export-oci generate-schedule prepare-oci-bundle test-analysis-input test-base1-skeleton test-base2-codegen test-base2-direct test-evaluator test-evaluator-image test-freezecheck test-nullable-compatibility test-rundriver test-runresult test-runresult-integration test-task7-dry-run validate-analysis-input validate-config validate-formal validate-oci-bundle validate-results validate-run validate-schedule validate-task-targets verify-task-solutions

build-evaluator:
	mkdir -p bin
	cd evaluator && go build -o ../bin/evaluator ./cmd/evaluator

build-evaluator-image:
	docker build --file images/evaluator.Dockerfile --tag specs-experiment-evaluator:go1.26.6 .

test-evaluator-image:
	./images/test-evaluator-image.sh

export-oci:
	./images/export-oci.sh

prepare-oci-bundle:
	test -n "$(OUTPUT_DIR)"
	./images/prepare-oci-bundle.sh "$(OUTPUT_DIR)"

validate-oci-bundle:
	test -n "$(BUNDLE_DIR)" && test -n "$(EVIDENCE_DIR)"
	./images/validate-oci-bundle.sh "$(BUNDLE_DIR)" "$(EVIDENCE_DIR)"

build-runresult:
	mkdir -p bin
	cd harness/runresult && go build -o ../../bin/runresult .

build-rundriver:
	mkdir -p bin
	cd harness/rundriver && go build -o ../../bin/rundriver .

build-analysis-input:
	mkdir -p bin
	cd analysis/input && go build -o ../../bin/analysis-input .

build-freezecheck:
	mkdir -p bin
	cd harness/freezecheck && go build -o ../../bin/freezecheck .

test-freezecheck:
	cd harness/freezecheck && go test ./...

validate-config:
	cd harness/freezecheck && go run . config --root ../..

generate-schedule:
	test -n "$(PHASE)" && test -n "$(SEED)" && test -n "$(REVISION)" && test -n "$(GENERATED_AT)" && test -n "$(OUTPUT)"
	cd harness/freezecheck && go run . schedule generate --config ../../config/experiment.json --phase "$(PHASE)" --seed "$(SEED)" --config-revision "$(REVISION)" --generated-at "$(GENERATED_AT)" --output "../../$(OUTPUT)"

validate-schedule:
	test -n "$(PHASE)" && test -n "$(SCHEDULE)"
	cd harness/freezecheck && go run . schedule validate --config ../../config/experiment.json --schedule "../../$(SCHEDULE)" --phase "$(PHASE)"

validate-run:
	test -n "$(RUN_DIR)"
	cd harness/freezecheck && go run . run --root ../.. --run-dir "../../$(RUN_DIR)" $(if $(SCHEDULE),--schedule "../../$(SCHEDULE)")

validate-results:
	test -n "$(RESULTS_DIR)"
	cd harness/freezecheck && go run . results --root ../.. --results-dir "../../$(RESULTS_DIR)" $(if $(SCHEDULE),--schedule "../../$(SCHEDULE)")

test-nullable-compatibility:
	cd compatibility/nullable && go test ./...

test-runresult:
	cd harness/runresult && go test ./...

test-runresult-integration: build-evaluator build-runresult
	./harness/test-runresult-integration.sh

test-rundriver:
	cd harness/rundriver && go test ./...

test-analysis-input:
	cd analysis/input && go test ./...

test-task7-dry-run: build-evaluator build-runresult build-rundriver build-freezecheck build-analysis-input
	./harness/test-task7-dry-run.sh

validate-analysis-input: build-analysis-input
	test -n "$(RESULTS_DIR)" && test -n "$(OUTPUT)"
	./bin/analysis-input -root . -results-dir "$(RESULTS_DIR)" -output "$(OUTPUT)"

test-evaluator:
	cd evaluator && go test ./...

evaluate-base2-direct:
	mkdir -p results/evaluator-baselines
	cd evaluator && go run ./cmd/evaluator -task baseline-service -candidate ../fixtures/base2-direct -output ../results/evaluator-baselines/base2-direct.json

evaluate-base2-codegen:
	mkdir -p results/evaluator-baselines
	cd evaluator && go run ./cmd/evaluator -task baseline-service -candidate ../fixtures/base2-codegen -output ../results/evaluator-baselines/base2-codegen.json

evaluate-bases: evaluate-base2-direct evaluate-base2-codegen

validate-task-targets:
	./tasks/propagation/validate-targets.sh

verify-task-solutions:
	./fixtures/verify-task-solutions.sh

evaluate-task-solutions:
	mkdir -p results/task-solutions
	cd evaluator && go run ./cmd/evaluator -task nullable-patch -candidate ../fixtures/task-solutions/nullable-patch-direct -output ../results/task-solutions/nullable-patch-direct.json
	cd evaluator && go run ./cmd/evaluator -task nullable-patch -candidate ../fixtures/task-solutions/nullable-patch-codegen -output ../results/task-solutions/nullable-patch-codegen.json
	cd evaluator && go run ./cmd/evaluator -task optimistic-locking -candidate ../fixtures/task-solutions/optimistic-locking-direct -output ../results/task-solutions/optimistic-locking-direct.json
	cd evaluator && go run ./cmd/evaluator -task optimistic-locking -candidate ../fixtures/task-solutions/optimistic-locking-codegen -output ../results/task-solutions/optimistic-locking-codegen.json
	cd evaluator && go run ./cmd/evaluator -task cursor-pagination -candidate ../fixtures/task-solutions/cursor-pagination-direct -output ../results/task-solutions/cursor-pagination-direct.json
	cd evaluator && go run ./cmd/evaluator -task cursor-pagination -candidate ../fixtures/task-solutions/cursor-pagination-codegen -output ../results/task-solutions/cursor-pagination-codegen.json

test-base1-skeleton:
	./fixtures/test-base1-skeleton.sh

test-base2-direct:
	./fixtures/test-base2-direct.sh

test-base2-codegen:
	./fixtures/test-base2-codegen.sh

validate-formal:
	./fixtures/validate-formal.sh
