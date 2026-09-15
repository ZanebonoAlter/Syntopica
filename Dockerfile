# Stage 1: Build frontend static files
FROM node:22-alpine AS front-build
WORKDIR /app
RUN corepack enable
COPY front/package.json front/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY front/ .
# 同源部署：这个镜像里后端（internal/app/static.go）自己提供前端静态产物，
# 浏览器与 API 必然同 origin，故注入**相对** base。
# 静态 SPA 的 runtimeConfig.public 在构建期内联 —— 运行期设同名环境变量无效，
# 必须在这里给；默认的 http://localhost:5100/api 会把访问主机烘死进产物，
# 非本机浏览器（局域网/远程）打开即 ERR_CONNECTION_REFUSED。
# 需要「前端与 API 不同 origin」时：--build-arg NUXT_PUBLIC_API_BASE=http://host:5100/api
ARG NUXT_PUBLIC_API_BASE=/api
ENV NUXT_PUBLIC_API_BASE=${NUXT_PUBLIC_API_BASE}
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
