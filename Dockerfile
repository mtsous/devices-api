# Build stage: compile a static binary.
FROM golang:1.23-alpine AS build

WORKDIR /src

# Download dependencies first so they are cached between code changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/devices-api ./cmd/devices-api

# Runtime stage: no shell or package manager, runs as a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/devices-api /devices-api

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/devices-api"]
