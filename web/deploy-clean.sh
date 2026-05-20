#!/bin/bash

# Clean Deployment Script - No hardcoded paths
# Uses package.json scripts for PM2 management

set -e

echo "🚀 Starting clean deployment..."

# Navigate to project directory (any directory)
PROJECT_PATH=$(pwd)
echo "📁 Project path: $PROJECT_PATH"

# Install dependencies
echo "📦 Installing dependencies..."
if command -v pnpm &> /dev/null; then
    pnpm install --frozen-lockfile --prod
else
    npm install --production
fi

# Build the application
echo "🔨 Building application..."
NODE_ENV=production npm run build

# Create logs directory
mkdir -p logs

# Stop existing processes
echo "🛑 Stopping existing PM2 processes..."
npm run pm2:stop 2>/dev/null || true

# Start the application
echo "🚀 Starting application with PM2..."
npm run pm2:start

# Save PM2 configuration
pm2 save

# Setup PM2 startup
pm2 startup systemd -u $(whoami) --hp $(eval echo ~$(whoami)) --silent || true

echo "✅ Deployment completed!"
echo "🌐 Application running at: http://localhost:3000"
echo "📊 Status: npm run pm2:status"
echo "📝 Logs: npm run pm2:logs"
echo "🔄 Restart: npm run pm2:restart"
