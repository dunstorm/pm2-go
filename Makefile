build:
	go build -o bin/main main.go

daemon:
	go build -o bin/main main.go
	./bin/main kill
	./bin/main -d

test:
	./bin/main start python test.py

ls:
	./bin/main ls

testindex:
	./bin/main start celery worker
	./bin/main stop 0