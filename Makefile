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
