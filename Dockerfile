FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bot ./cmd/bot

FROM gcr.io/distroless/static:nonroot
COPY --from=build /bot /bot
COPY internal/persona/default.md /persona/default.md
EXPOSE 8080
ENTRYPOINT ["/bot"]
