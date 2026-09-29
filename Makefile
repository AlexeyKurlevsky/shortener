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

pprof_memory:
	go tool pprof -http=":9090" -seconds=60 heap.out
	go tool pprof -http=":9090" -sample_index=alloc_space http://localhost:8081/debug/pprof/heap
pprof_cpu:
	go tool pprof -http=":9090" -seconds=30 http://localhost:8081/debug/pprof/profile

pprof_top:
	go tool pprof -proto -output=base.pprof http://localhost:8081/debug/pprof/heap

bench_test:
	go test -bench=. -benchmem ./internal/handlers

lint_imports:
	goimports -local "github.com/AlexeyKurlevsky/shortener" -w .

doc:
	go doc ./internal/handlers/