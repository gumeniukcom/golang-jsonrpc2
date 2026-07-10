all: easy fmt lint test

GOFILES = ./...

easy:
	cd structs && easyjson -all -pkg

fmt:
	go fmt $(GOFILES)

lint:
	golangci-lint run $(GOFILES)

test:
	go test $(GOFILES)

testv:
	go test -v $(GOFILES)

testrace:
	go test -race $(GOFILES)

BENCHSTAT := golang.org/x/perf/cmd/benchstat@v0.0.0-20260709024250-82a0b07e230d

bench:
	go test -run xxx -bench=. -benchmem $(GOFILES)

# Cross-library comparison suite (benchmarks/ nested module).
bench-compare:
	cd benchmarks && go test -run xxx -bench=. -benchmem -count=10 ./...

bench-save:
	cd benchmarks && go test -run xxx -bench=. -benchmem -count=10 ./... | tee results/$$(date +%F).txt

# Usage: make benchstat OLD=benchmarks/results/a.txt NEW=benchmarks/results/b.txt
benchstat:
	go run $(BENCHSTAT) $(OLD) $(NEW)

cover:
	go test -race -coverprofile=coverage.out -covermode=atomic $(GOFILES)
	go tool cover -func=coverage.out | tail -1

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 $(GOFILES)
