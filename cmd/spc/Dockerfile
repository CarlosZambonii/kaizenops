# syntax=docker/dockerfile:1
#
# Build context MUST be the repository root:
#   docker build -f cmd/spc/Dockerfile .
#
# This is required because the module root (go.mod/go.sum) and all of
# internal/ and pkg/ must be visible to the build stage.

FROM golang:1.25 AS builder
WORKDIR /src

# Cache dependency downloads in their own layer.
COPY go.mod go.sum ./
RUN go mod download

# Now copy the rest of the module and build.
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/spc

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=builder /out/app /app
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app"]
