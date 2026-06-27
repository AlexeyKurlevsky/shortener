run:
	go run ./cmd/shortener/

build:
	go build -o ./cmd/shortener/shortener ./cmd/shortener

test_course: build
	shortenertest -test.v -test.run=^TestIteration4$ -binary-path=cmd/shortener/shortener

test:
	go test -v ./cmd/shortener