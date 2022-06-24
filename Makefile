build:
	go build -o bin/main main.go

daemon:
	go build -o bin/main main.go
	./bin/main kill
	./bin/main -d

start:
	./bin/main start examples/ecosystem.json

stop:
	./bin/main stop examples/ecosystem.json

delete:
	./bin/main delete examples/ecosystem.json

ls:
	./bin/main ls

kill:
	./bin/main kill

logs:
	./bin/main logs python-test

test:
	go test -v

dump:
	./bin/main dump

restore:
	./bin/main restore

flush:
	./bin/main flush