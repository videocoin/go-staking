IMAGE = registry.videocoin.net/staking/setter
VERSION ?= $(shell git describe --tags)

.PHONY: generate
generate:
	go generate ./...

.PHONY: deps
deps:
	go get -mod=readonly golang.org/x/tools/cmd/stringer


.PHONY: build
build:
	mkdir -p ./build
	go build -mod=vendor -o ./build/setter ./cmd

.PHONY: vendor
vendor:
	go mod vendor
	modvendor -copy="**/*.c **/*.h"

.PHONY: image
image:
	docker build -t ${IMAGE}:${VERSION} -f _assets/Dockerfile .

.PHONY: push
push:
	docker push ${IMAGE}:${VERSION}
