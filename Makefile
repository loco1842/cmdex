.PHONY: dev build generate check fmt lint test clean

dev:
	wails3 dev

build:
	wails3 build

generate:
	wails3 generate bindings

check:
	mkdir -p frontend/dist
	go build ./...
	cd frontend && pnpm tsc --noEmit

fmt:
	golangci-lint fmt

lint:
	golangci-lint run || true

lint-fix:
	golangci-lint run --fix || true

test:
	mkdir -p frontend/dist
	go test ./...
	cd frontend && pnpm test:e2e

clean:
	rm -rf bin frontend/dist
	mkdir -p frontend/dist
	touch frontend/dist/.gitkeep
