FROM golang:1.22.5-alpine as builder

WORKDIR /app

COPY . .

RUN go build -tags netgo -ldflags '-s -w' -o ./watcher ./cmd/local/local.go

FROM alpine:latest 

COPY --from=builder /app/matcher  .

COPY  --from=builder /app/*.json .

CMD ["./watcher"]