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

testindex:
	./bin/main start celery worker
	./bin/main stop 0