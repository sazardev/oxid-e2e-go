# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS build
WORKDIR /src
ENV GOFLAGS=-mod=mod CGO_ENABLED=0
COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download || true
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -ldflags="-s -w" -o /out/app .

FROM alpine:3.19
COPY --from=build /out/app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
