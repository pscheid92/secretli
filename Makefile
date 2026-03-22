.PHONY: dev dev-api dev-frontend build build-frontend build-go test test-short test-coverage clean lint lint-go lint-frontend

# Run both backend and frontend in development mode
dev:
	$(MAKE) -j2 dev-api dev-frontend

# Backend with auto-reload via Air
dev-api:
	air -c .air.toml

# Frontend Vite dev server with proxy to Go backend
dev-frontend:
	cd web/frontend && pnpm dev

# Production build
build: build-frontend build-go

# Build frontend
build-frontend:
	cd web/frontend && pnpm install --frozen-lockfile && pnpm build

# Build Go binary (requires frontend to be built first)
build-go:
	CGO_ENABLED=0 go build -o bin/secretli .

# Fast unit tests only (no containers)
test-short:
	go test -short -cover ./...
	cd web/frontend && pnpm test --run

# Full test suite including integration tests
test:
	go test -cover ./...
	cd web/frontend && pnpm test --run

# Generate coverage reports
test-coverage:
	go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out
	cd web/frontend && pnpm test:coverage

# Lint all
lint: lint-go lint-frontend

# Lint Go
lint-go:
	golangci-lint run ./...

# Lint frontend
lint-frontend:
	cd web/frontend && npx biome check .

# Clean build artifacts
clean:
	rm -rf bin/ tmp/ web/frontend/dist/
