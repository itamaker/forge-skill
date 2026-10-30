BINARY := skillforge

.PHONY: build test example release-check snapshot

build:
	mkdir -p dist
	go build -o dist/$(BINARY) .

test:
	go test ./...

example:
	go run . init -spec examples/skill.json -out /tmp/researchops -force

release-check:
	goreleaser check

snapshot:
	goreleaser release --snapshot --clean

