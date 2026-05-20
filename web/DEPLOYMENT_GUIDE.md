# ZGI Web PM2 Deployment Guide

## Server Setup

### Prerequisites

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install Node.js 20
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt-get install -y nodejs

# Install pnpm
npm install -g pnpm@10.12.1

# Install PM2
npm install -g pm2

# Install required system packages
sudo apt install -y build-essential python3 make g++
```

## Deployment Steps

### 1. Navigate to Project Directory

```bash
cd /root/item/zgi-web
```

### 2. Make Deploy Script Executable

```bash
chmod +x deploy.sh
```

### 3. Run Deployment

```bash
./deploy.sh
```

### 4. Manual Deployment (Alternative)

```bash
# Install dependencies
pnpm install --frozen-lockfile --prod

# Build application
pnpm build

# Start with PM2
pm2 start ecosystem.config.js

# Save PM2 configuration
pm2 save

# Setup PM2 to start on boot
pm2 startup
sudo env PATH=$PATH:/usr/bin /usr/lib/node_modules/pm2/bin/pm2 startup systemd -u root --hp /root
```

## Environment Configuration

Copy and configure the production environment file:

```bash
cp .env.production .env.local
```

Update the following variables in `.env.local`:

- `CONSOLE_API_URL` - Your backend API URL
- `APP_API_URL` - Your app API URL  
- `MARKETPLACE_API_URL` - Your marketplace API URL
- `MARKETPLACE_URL` - Your marketplace frontend URL

## PM2 Management Commands

### View Application Status

```bash
pm2 status
```

### View Logs

```bash
pm2 logs zgi-web
```

### Restart Application

```bash
pm2 restart zgi-web
```

### Stop Application

```bash
pm2 stop zgi-web
```

### Delete Application

```bash
pm2 delete zgi-web
```

### Monitor Performance

```bash
pm2 monit
```

## Firewall Configuration

Ensure port 3000 is open:
```bash
sudo ufw allow 3000
sudo ufw reload
```

## Nginx Reverse Proxy (Optional)

Create Nginx configuration:
```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
    }
}
```

## Troubleshooting

### Check if Application is Running

```bash
curl http://localhost:3000
```

### Check PM2 Logs for Errors

```bash
pm2 logs zgi-web --err
```

### Rebuild Application

```bash
cd /root/item/zgi-web
pnpm build
pm2 restart zgi-web
```

### Update Application

```bash
cd /root/item/zgi-web
git pull origin main
pnpm install --frozen-lockfile --prod
pnpm build
pm2 restart zgi-web
```

## Performance Monitoring

The application is configured with:

- Cluster mode with 2 instances
- Memory limit of 1GB per instance
- Automatic restart on memory overflow
- Log rotation

Monitor performance with:

```bash
pm2 monit
```

## SSL Certificate (Optional)

For HTTPS, configure SSL with Let's Encrypt:

```bash
sudo apt install certbot python3-certbot-nginx
sudo certbot --nginx -d your-domain.com
```
