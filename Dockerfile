# syntax=docker/dockerfile:experimental

ARG AVALANCHEGO_COMMIT
FROM golang:1.15.5-buster

ARG AVALANCHEGO_COMMIT

RUN mkdir -p /go/src/github.com/ava-labs

WORKDIR $GOPATH/src/github.com/ava-labs/
COPY . avalanchego

WORKDIR $GOPATH/src/github.com/ava-labs/avalanchego
RUN export AVALANCHEGO_COMMIT=$AVALANCHEGO_COMMIT
RUN ./scripts/build.sh

RUN ln -sv $GOPATH/src/github.com/ava-labs/avalanchego/ /avalanchego
