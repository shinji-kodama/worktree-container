ARG NODE_VERSION=20
FROM node:${NODE_VERSION}-slim

RUN apt-get update && apt-get install -y git curl && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
