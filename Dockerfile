FROM golang:1.21-alpine as builder

RUN mkdir /app
WORKDIR /app
COPY . .
ARG PAT
RUN apk add git 
RUN git config --global url."https://${PAT}:@github.com/".insteadOf "https://github.com/"
RUN go get github.com/catalogfi/orderbook
RUN go build -tags netgo -ldflags '-s -w' -o ./cobi ./cmd/docker/main.go

FROM alpine:latest  
ENV GOPRIVATE github.com/catalogfi/orderbook
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/cobi  .
CMD ["./cobi"]