FROM golang:1.22

RUN apt-get update && apt-get install -y \
    git \
    curl \
    unzip \
    vim \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

ENV GO111MODULE=on
