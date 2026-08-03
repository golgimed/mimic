.PHONY: build test lint fmt fmt-fix vet vulncheck check run docker release-snapshot clean

build:
	go build -o bin/mimic ./cmd/mimic

run: build
	./bin/mimic

test:
	go test -race ./...

fmt:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

fmt-fix:
	gofmt -w .

vet:
	go vet ./...

lint:
	golangci-lint run

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

check: fmt vet lint test vulncheck

docker:
	docker build -f docker/Dockerfile -t mimic:dev .

release-snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -rf bin dist
