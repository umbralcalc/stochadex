# syntax=docker/dockerfile:1

FROM golang:1.21

WORKDIR /stochadex

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

# build the stochadex binary
RUN go build -o bin/ ./cmd/stochadex

# expose the websocket port
EXPOSE 2112

ENTRYPOINT ["./bin/stochadex"]