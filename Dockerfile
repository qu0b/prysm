FROM golang:1.23.2-bookworm AS builder

# build deps
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    curl \
    libpcre3-dev \
    lsb-release \
    software-properties-common \
    apt-transport-https \
    ca-certificates \
    wget \
    tzdata \
    bash \
    make \
    g++ \
    gnupg \
    cmake \
    libc6 \
    libc6-dev \
    libsnappy-dev \
    gradle \
    pkg-config \
    libssl-dev \
    libclang-dev

RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && \
apt-get install -y nodejs

RUN npm install -g @bazel/bazelisk

ENV CGO_ENABLED=1

RUN go install github.com/antithesishq/antithesis-sdk-go/tools/antithesis-go-instrumentor@latest

COPY ./ /prysm

WORKDIR /prysm

RUN go mod tidy

RUN antithesis-go-instrumentor -assert_only -catalog_dir=./cmd/beacon-chain ./ 

RUN bazelisk build --config=release //cmd/beacon-chain:beacon-chain

FROM debian:stable-slim

COPY --from=builder /prysm/bazel-bin/cmd/beacon-chain/beacon-chain_/beacon-chain /usr/local/bin/beacon-chain

ENTRYPOINT /usr/local/bin/beacon-chain

# https://github.com/ethpandaops/eth-client-docker-image-builder/blob/master/prysm/Dockerfile.beacon