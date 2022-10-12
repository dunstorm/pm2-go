protoc:
	@echo "Generating Go files"
	cd proto && protoc --go_out=. --go-grpc_out=. *.proto

build:
	go build -o bin/pm2-go main.go

install:
	go install .

daemon:
	go build -o bin/pm2-go main.go
	./bin/pm2-go kill
	./bin/pm2-go -d

start:
	./bin/pm2-go start examples/ecosystem.json

stop:
	./bin/pm2-go stop examples/ecosystem.json

delete:
	./bin/pm2-go delete examples/ecosystem.json

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
