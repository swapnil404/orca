FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY pkg ./pkg
COPY server ./server
RUN CGO_ENABLED=0 go build -o /out/orca-server ./server/cmd/server

FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && addgroup -S orca \
    && adduser -S -G orca orca
COPY --from=build /out/orca-server /usr/local/bin/orca-server

USER orca
EXPOSE 8080
ENTRYPOINT ["orca-server"]
