DOCKER ?= docker
E2E_IMAGE ?= pm2-go-e2e
BENCHMARK_IMAGE ?= pm2-go-benchmark

.PHONY: protoc build build/cli build/web install daemon web/dev test/quick/start test/quick/stop ls kill logs test test/e2e test/e2e/slow test/e2e/docker test/e2e/docker/slow benchmark benchmark/docker dump restore flush

protoc:
	@echo "Generating Go files"
	cd proto && protoc --go_out=. --go-grpc_out=. *.proto

build: build/cli build/web

build/cli:
	go build -o bin/pm2-go ./cmd/pm2-go

build/web:
	go build -o bin/pm2-go-web ./cmd/pm2-go-web

install:
	go install ./cmd/pm2-go ./cmd/pm2-go-web

web/dev: build/cli
	PM2_GO_WEB_TOKEN="$${PM2_GO_WEB_TOKEN:-dev-token}" ./bin/pm2-go web --port $${PM2_GO_WEB_PORT:-9615} --read-only --dev-assets internal/web --dev-reload

daemon:
	go build -o bin/pm2-go ./cmd/pm2-go
	./bin/pm2-go kill
	./bin/pm2-go -d

test/quick/start:
	go build -o bin/pm2-go ./cmd/pm2-go
	./bin/pm2-go start examples/ecosystem.json

test/quick/stop:
	./bin/pm2-go stop examples/ecosystem.json
	./bin/pm2-go delete examples/ecosystem.json
	./bin/pm2-go kill

ls:
	./bin/pm2-go ls

kill:
	./bin/pm2-go kill

logs:
	./bin/pm2-go logs python-test

test:
	go test -v ./...

test/e2e:
	./scripts/e2e.sh

test/e2e/slow:
	E2E_SLOW=1 ./scripts/e2e.sh

test/e2e/docker:
	$(DOCKER) build -f docker/e2e.Dockerfile -t $(E2E_IMAGE) .
	$(DOCKER) run --rm \
		-v "$(CURDIR)":/workspace \
		-v pm2-go-go-mod-cache:/go/pkg/mod \
		-v pm2-go-go-build-cache:/root/.cache/go-build \
		-w /workspace \
		$(E2E_IMAGE) ./scripts/e2e.sh

test/e2e/docker/slow:
	$(DOCKER) build -f docker/e2e.Dockerfile -t $(E2E_IMAGE) .
	$(DOCKER) run --rm \
		-e E2E_SLOW=1 \
		-v "$(CURDIR)":/workspace \
		-v pm2-go-go-mod-cache:/go/pkg/mod \
		-v pm2-go-go-build-cache:/root/.cache/go-build \
		-w /workspace \
		$(E2E_IMAGE) ./scripts/e2e.sh

benchmark:
	./scripts/benchmark_pm2_vs_pm2_go.py

benchmark/docker:
	$(DOCKER) build -f docker/benchmark.Dockerfile -t $(BENCHMARK_IMAGE) .
	$(DOCKER) run --rm \
		-v "$(CURDIR)":/workspace \
		-v pm2-go-go-mod-cache:/go/pkg/mod \
		-v pm2-go-go-build-cache:/root/.cache/go-build \
		-w /workspace \
		$(BENCHMARK_IMAGE) ./scripts/benchmark_pm2_vs_pm2_go.py

dump:
	./bin/pm2-go dump

restore:
	./bin/pm2-go restore

flush:
	./bin/pm2-go flush
