FROM golang:1.24-alpine AS build
WORKDIR /src

COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/worker ./cmd/worker

FROM alpine:3.21
RUN apk add --no-cache ca-certificates poppler-utils tzdata
RUN adduser -D -H -u 10001 appuser
COPY --from=build /out/worker /usr/local/bin/worker
USER appuser
EXPOSE 8082
ENTRYPOINT ["/usr/local/bin/worker"]
