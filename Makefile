run:
	go run ./cmd/shortener/

build:
	go build -o ./cmd/shortener/shortener ./cmd/shortener

test_course: build
	shortenertest -test.v -test.run=^TestIteration10$ -binary-path=cmd/shortener/shortener

test:
	go test -v ./...

migrate:
	migrate create -ext sql -dir ./internal/storage/migrations -seq create_users_table