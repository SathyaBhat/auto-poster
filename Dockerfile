FROM golang:1.27-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /auto-poster ./cmd/auto-poster

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app && mkdir /data && chown app /data
USER app
COPY --from=builder /auto-poster /usr/local/bin/auto-poster
ENV DB_PATH=/data/auto-poster.db
VOLUME /data
ENTRYPOINT ["auto-poster"]
