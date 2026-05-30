# Stage 1: Build frontend static files
FROM node:22-alpine AS front-build
WORKDIR /app
RUN corepack enable
COPY front/package.json front/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY front/ .
RUN pnpm generate

# Stage 2: Runtime image
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 appuser

WORKDIR /app

# Copy pre-built Go binary (user builds locally)
ARG BINARY_PATH=./backend-go/syntopica
COPY ${BINARY_PATH} /app/syntopica

# Copy backend configs
COPY backend-go/configs /app/configs

# Copy frontend static files from build stage
COPY --from=front-build /app/.output/public/ /app/frontend/

USER appuser

ENV SERVER_PORT=5000 SERVER_MODE=release
EXPOSE 5000

CMD ["/app/syntopica"]
