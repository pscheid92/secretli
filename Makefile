.PHONY: dev dev-api dev-frontend build build-frontend build-go test clean

# Run both backend and frontend in development mode
dev:
	$(MAKE) -j2 dev-api dev-frontend

# Backend with auto-reload via Air
dev-api:
	air -c .air.toml

# Frontend Vite dev server with proxy to Go backend
dev-frontend:
	cd web/frontend && npm run dev

# Production build
build: build-frontend build-go

# Build frontend
build-frontend:
	cd web/frontend && npm ci && npm run build

# Build Go binary (requires frontend to be built first)
build-go:
	CGO_ENABLED=0 go build -o bin/secretli .

# Run all tests
test:
	go test ./...
	cd web/frontend && npm test

# Clean build artifacts
clean:
	rm -rf bin/ tmp/ web/frontend/dist/
