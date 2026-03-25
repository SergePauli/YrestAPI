FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN mkdir -p /out/cfg \
    && cp -R def_cfg/. /out/cfg/ \
    && if [ -d cfg ]; then cp -R cfg/. /out/cfg/; fi

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/yrestapi ./cmd

FROM gcr.io/distroless/base-debian12

WORKDIR /app

COPY --chown=65532:65532 --from=build /out/yrestapi /app/yrestapi
COPY --chown=65532:65532 db /app/db
COPY --chown=65532:65532 test_db /app/test_db
COPY --chown=65532:65532 --from=build /out/cfg /app/cfg
COPY --chown=65532:65532 log /app/log

EXPOSE 8080

USER 65532:65532

ENTRYPOINT ["/app/yrestapi"]
