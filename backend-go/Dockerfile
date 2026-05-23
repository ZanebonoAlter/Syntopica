FROM golang:1.25-alpine AS build

WORKDIR /src

ARG GOPROXY
ARG GOSUMDB
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY

COPY go.mod go.sum ./
RUN if [ -n "$GOPROXY" ]; then go env -w GOPROXY="$GOPROXY"; fi \
  && if [ -n "$GOSUMDB" ]; then go env -w GOSUMDB="$GOSUMDB"; fi \
  && go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/syntopica ./cmd/server

FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata \
  && adduser -D -u 10001 appuser \
  && mkdir -p /app/data /app/configs \
  && chown -R appuser:appuser /app

COPY --from=build /out/syntopica /app/syntopica
COPY configs /app/configs

USER appuser

ENV SERVER_PORT=5000 \
  SERVER_MODE=release \
  DATABASE_DSN=/app/data/syntopica.db

EXPOSE 5000

CMD ["/app/syntopica"]
