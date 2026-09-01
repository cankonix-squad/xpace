FROM golang:1.27.0-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/xpace-api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -o /out/recording-smoke ./cmd/recording-smoke

FROM alpine:3.24
RUN apk upgrade --no-cache && \
    addgroup -S xpace && adduser -S -G xpace xpace
COPY --from=build /out/xpace-api /usr/local/bin/xpace-api
COPY --from=build /out/recording-smoke /usr/local/bin/recording-smoke
USER xpace
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/xpace-api"]
