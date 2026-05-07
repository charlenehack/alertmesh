FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/alertmesh ./cmd/alertmesh

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/alertmesh /usr/local/bin/alertmesh

EXPOSE 8080

ENTRYPOINT ["alertmesh"]
