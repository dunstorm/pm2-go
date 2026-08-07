FROM golang:1.25-alpine

RUN apk add --no-cache bash ca-certificates procps python3

WORKDIR /workspace
