#!/bin/bash

# Clean up old generated files
rm -f api/*.pb.go

# Build the Docker image
docker build -t protoc-compiler -f Dockerfile.protoc .

# Run the container with volume mount to share the generated files
docker run --rm -v $(pwd):/app protoc-compiler

echo "Proto files have been compiled successfully!" 