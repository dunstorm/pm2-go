FROM golang:1.23-alpine

RUN apk add --no-cache bash ca-certificates procps python3

WORKDIR /workspace
