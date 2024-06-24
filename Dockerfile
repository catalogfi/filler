FROM golang:1.21 as builder

RUN mkdir /app
WORKDIR /app

COPY . .

ARG PAT
ENV GOPRIVATE github.com/catalogfi/orderbook
RUN git config --global url."https://${PAT}:@github.com/".insteadOf "https://github.com/"

RUN go get github.com/catalogfi/orderbook
RUN go build -tags netgo -ldflags '-s -w' -o ./cobi ./cmd/docker/main.go

FROM alpine:latest  
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/cobi    .
CMD ["./cobi"]