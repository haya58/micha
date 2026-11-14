FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bot ./cmd/bot

# Alpine (not distroless): exec_shell needs the `docker` CLI to drive the
# DinD sidecar over TCP ($DOCKER_HOST). The Go binary is static.
FROM alpine:3.21
RUN apk add --no-cache docker-cli
COPY --from=build /bot /bot
COPY internal/persona/default.md /persona/default.md
USER nobody
EXPOSE 8080
ENTRYPOINT ["/bot"]
