FROM node:24-bookworm-slim AS frontend-build

WORKDIR /src/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend ./
RUN npm run build

FROM golang:1.25-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend-build /src/internal/webui/dist ./internal/webui/dist

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -o /out/app ./cmd/server

FROM debian:bookworm-slim

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /app/data

COPY --from=build /out/app /app/app

ENV PORT=8080
ENV SQLITE_PATH=/app/data/nitrolite-store-example.db
ENV STORE_NAME="Nitrolite App Session Store"
ENV STORE_APP_ID=default
ENV LOG_LEVEL=info

EXPOSE 8080

CMD ["/app/app"]
