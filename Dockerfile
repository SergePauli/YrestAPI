FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/yrestapi ./cmd

FROM gcr.io/distroless/base-debian12

WORKDIR /app

COPY --chown=65532:65532 --from=build /out/yrestapi /app/yrestapi
COPY --chown=65532:65532 db /app/db
COPY --chown=65532:65532 cfg /app/cfg
COPY --chown=65532:65532 log /app/log

ENV MODELS_DIR=/app/db
EXPOSE 8080

USER 65532:65532

ENTRYPOINT ["/app/yrestapi"]
