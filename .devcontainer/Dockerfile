# Use Ubuntu as the base image
FROM ubuntu:22.04

# Install essential packages
RUN apt-get update && \
    apt-get install -y build-essential gcc make libjson-c-dev git && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Set up a non-root user (optional, for VS Code devcontainers best practices)
ARG USERNAME=vscode
ARG USER_UID=1000
ARG USER_GID=$USER_UID
RUN groupadd --gid $USER_GID $USERNAME \
    && useradd --uid $USER_UID --gid $USER_GID -m $USERNAME \
    && apt-get update && apt-get install -y sudo \
    && echo "$USERNAME ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers.d/$USERNAME \
    && chmod 0440 /etc/sudoers.d/$USERNAME

USER $USERNAME
WORKDIR /workspace
