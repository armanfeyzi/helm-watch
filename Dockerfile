# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/helm-watch ./cmd/helm-watch

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=builder /out/helm-watch /helm-watch

EXPOSE 8080
ENTRYPOINT ["/helm-watch"]
