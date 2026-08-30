.PHONY: dev build generate check fmt lint lint-fix test clean

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
	cd frontend && pnpm lint:fix

lint:
	golangci-lint run
	cd frontend && pnpm lint

lint-fix:
	golangci-lint run --fix
	cd frontend && pnpm lint:fix

test:
	mkdir -p frontend/dist
	go test ./...
	cd frontend && pnpm test
	cd frontend && pnpm test:e2e

clean:
	rm -rf bin frontend/dist
	mkdir -p frontend/dist
	touch frontend/dist/.gitkeep
