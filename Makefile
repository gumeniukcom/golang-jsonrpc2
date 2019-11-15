all: easy fmt test metalint

GOFILES=`go list ./... | grep -v vendor`

metalint:
	golint $(GOFILES)
	gometalinter ./... --config=.gometalinter.json

easy:
	easyjson --all structs/

fmt:
	go fmt $(GOFILES)
test:
	GO111MODULE=on go test -mod vendor $(GOFILES)
testv:
	GO111MODULE=on go test -v -mod vendor $(GOFILES)
testv:
	GO111MODULE=on go test -v -mod vendor $(GOFILES)