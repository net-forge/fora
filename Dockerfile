FROM golang:1.23-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/hive-server ./hive-server

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/hive-server /usr/local/bin/hive-server

VOLUME ["/data", "/keys"]
EXPOSE 8080

ENTRYPOINT ["hive-server"]
CMD ["--port", "8080", "--db", "/data/hive.db"]
