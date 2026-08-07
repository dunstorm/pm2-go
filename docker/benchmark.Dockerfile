FROM golang:1.25-alpine

ARG PM2_VERSION=latest

RUN apk add --no-cache bash ca-certificates nodejs npm procps python3 \
	&& npm install -g pm2@${PM2_VERSION}

WORKDIR /workspace
