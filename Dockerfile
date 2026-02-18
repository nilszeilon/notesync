# Build stage
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /notesync-server ./cmd/server

# Runtime stage
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /notesync-server /usr/local/bin/notesync-server
ENTRYPOINT ["notesync-server"]
CMD ["-port", "8080", "-data", "/data", "-site", "/_site"]
