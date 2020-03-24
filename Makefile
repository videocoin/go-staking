.PHONY: generate
generate:
	go generate ./...

.PHONY: deps
deps:
	go get -mod=readonly golang.org/x/tools/cmd/stringer
