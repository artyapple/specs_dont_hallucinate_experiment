.PHONY: build-evaluator build-evaluator-image build-runresult evaluate-bases evaluate-base2-codegen evaluate-base2-direct evaluate-task-solutions export-oci test-base1-skeleton test-base2-codegen test-base2-direct test-evaluator test-evaluator-image test-runresult test-runresult-integration validate-formal validate-task-targets verify-task-solutions

build-evaluator:
	mkdir -p bin
	cd evaluator && go build -o ../bin/evaluator ./cmd/evaluator

build-evaluator-image:
	docker build --file images/evaluator.Dockerfile --tag specs-experiment-evaluator:go1.26.6 .

test-evaluator-image:
	./images/test-evaluator-image.sh

export-oci:
	./images/export-oci.sh

build-runresult:
	mkdir -p bin
	cd harness/runresult && go build -o ../../bin/runresult .

test-runresult:
	cd harness/runresult && go test ./...

test-runresult-integration: build-evaluator build-runresult
	./harness/test-runresult-integration.sh

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
