# Dev Container for OPL DNS Plugin
FROM ubuntu:24.04

# Install build dependencies
RUN apt-get update && \
    apt-get install -y build-essential libjson-c-dev curl pkg-config bind9-dev && \
    rm -rf /var/lib/apt/lists/*

# Set up working directory
WORKDIR /workspace

# Optionally copy source code (uncomment if needed)
# COPY . /workspace

CMD ["/bin/bash"]
