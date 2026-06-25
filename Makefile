run:
	go run ./cmd/shortener/main.go

build:
	go build -o shortener ./cmd/shortener

test_course: build
	shortenertest_v2 -test.v -test.run=^TestIteration1$ -binary-path=cmd/shortener/shortener

test:
	go test -v ./cmd/shortener