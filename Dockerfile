# Stage 1: Build frontend
FROM node:24-alpine AS frontend
RUN corepack enable pnpm
WORKDIR /app/web/frontend
COPY web/frontend/package.json web/frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/frontend/ ./
RUN pnpm build

# Stage 2: Build Go binary
FROM golang:1.26-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/frontend/dist ./web/frontend/dist
RUN CGO_ENABLED=0 go build -o /secretli .

# Stage 3: Final minimal image
FROM gcr.io/distroless/static-debian12
COPY --from=backend /secretli /secretli
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/secretli"]
