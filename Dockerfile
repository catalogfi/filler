# Dockerfile
FROM golang:1.21-alpine as baseImage

RUN mkdir app

WORKDIR /app

# Copy the Go app files
COPY  . .

# Build the Go app
RUN go build -tags netgo -ldflags '-s -w' -o ./cobi ./cmd/cobid

FROM alpine:latest

# Copy the built Go app from the previous stage
COPY --from=baseImage /app/cobi /app/cobi

# Set the entrypoint
ENTRYPOINT ["/app/cobi"]
