FROM debian:stable-slim AS etb-client-builder

# build deps
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    curl \
    libpcre3-dev \
    lsb-release \
    software-properties-common \
    apt-transport-https \
    openjdk-17-jdk \
    ca-certificates \
    wget \
    tzdata \
    bash \
    python3-dev \
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
    git \
    git-lfs \
    librocksdb7.8 \
    libclang-dev

# set up go (geth+prysm)
RUN arch=$(arch | sed s/aarch64/arm64/ | sed s/x86_64/amd64/) && \
    wget https://go.dev/dl/go1.21.5.linux-${arch}.tar.gz

# setup nodejs (lodestar)
RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash - && \
apt-get install -y nodejs

RUN npm install -g @bazel/bazelisk # prysm build system


FROM etb-client-builder AS prysm-builder
ARG PRYSM_BRANCH
ARG PRYSM_REPO
RUN git clone "${PRYSM_REPO}"; \
    cd prysm && git checkout "${PRYSM_BRANCH}"; \
    git log -n 1 --format=format:"%H" > /prysm.version

FROM prysm-builder AS prysm
RUN cd prysm && \
    bazelisk build --config=release //cmd/beacon-chain:beacon-chain //cmd/validator:validator

FROM prysm-builder AS prysm-race
RUN cd prysm && \
    bazelisk build --config=release --@io_bazel_rules_go//go/config:race //cmd/beacon-chain:beacon-chain //cmd/validator:validator


FROM debian:stable-slim

COPY --from=prysm-builder /git/prysm/bazel-bin/cmd/beacon-chain/beacon-chain_/beacon-chain /usr/local/bin/beacon-chain

COPY --from=prysm-builder /git/prysm/bazel-bin/cmd/validator/validator_/validator /usr/local/bin/validator


# https://github.com/ethpandaops/eth-client-docker-image-builder/blob/master/prysm/Dockerfile.beacon