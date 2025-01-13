#!/bin/bash

# Exit on any error
set -e

# Load environment variables
source .env

echo "Starting deployment..."

# Pull latest changes
git pull origin main

# Build and start containers
docker-compose pull
docker-compose up -d --build

# Check if containers are running
if docker-compose ps | grep -q "Up"; then
    echo "Deployment successful!"
else
    echo "Deployment failed. Check container logs."
    docker-compose logs
    exit 1
fi

# Clean up old images
docker image prune -f

echo "Deployment completed!" 