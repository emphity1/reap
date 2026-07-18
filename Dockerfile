FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /reap ./cmd/reap

# Distroless: no shell, no package manager — just the static binary. The
# tool reads local files and prints results, so nothing else is needed.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /reap /reap
ENTRYPOINT ["/reap"]
