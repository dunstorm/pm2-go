protoc:
	@echo "Generating Go files"
	cd proto && protoc --go_out=. --go-grpc_out=. *.proto

build: # Builds for the current OS by default, or for Linux if GOOS is not set
	go build -o bin/pm2-go main.go

build-windows:
	GOOS=windows GOARCH=amd64 go build -o bin/pm2-go.exe main.go

install:
	go install .

daemon:
	go build -o bin/pm2-go main.go
	./bin/pm2-go kill
	./bin/pm2-go -d

test/quick/start:
	go build -o bin/pm2-go main.go
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
	go test -v

dump:
	./bin/pm2-go dump

restore:
	./bin/pm2-go restore

flush:
	./bin/pm2-go flush

release:
	goreleaser release --snapshot --clean

.PHONY: protoc test ls kill dump restore flush build-windows release
