FROM golang:1.24-alpine

RUN apk add --no-cache bash ca-certificates procps python3

WORKDIR /workspace
