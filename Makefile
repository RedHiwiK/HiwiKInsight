VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all web build test vet demo docker clean

all: web build

## web: build the dashboard into web/dist/app (embedded into the server binary)
web:
	cd web && pnpm install --frozen-lockfile && pnpm build

## build: build the server and the CLI into ./bin
build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/hiwikinsight ./cmd/hiwikinsight
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/insight ./cmd/insight

## test: run Go tests and type-check the dashboard
test: vet
	go test ./...
	cd web && pnpm typecheck

vet:
	go vet ./...

## demo: run the demo (sample apps + generated data) at http://localhost:8080/dashboard/ (demo / demo)
demo: web
	go run ./cmd/hiwikinsight demo -data ./demo-data

## docker: build the container image
docker:
	docker build --build-arg VERSION=$(VERSION) -t hiwikinsight:$(VERSION) .

clean:
	rm -rf bin web/dist/app demo-data
