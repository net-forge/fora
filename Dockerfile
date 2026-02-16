FROM golang:1.23-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/fora-server ./fora-server

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/fora-server /usr/local/bin/fora-server

VOLUME ["/data", "/keys"]
EXPOSE 8080

ENTRYPOINT ["fora-server"]
CMD ["--port", "8080", "--db", "/data/fora.db"]
