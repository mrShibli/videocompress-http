# 🚀 Complete VPS Setup Guide: VideoCompress on AlmaLinux with Webmin

## 📋 Table of Contents
1. [Prerequisites](#prerequisites)
2. [Initial Server Setup](#initial-server-setup)
3. [Install Node.js and Dependencies](#install-nodejs-and-dependencies)
4. [Install FFmpeg](#install-ffmpeg)
5. [Setup VideoCompress Application](#setup-videocompress-application)
6. [Configure Firewall](#configure-firewall)
7. [Setup PM2 Process Manager](#setup-pm2-process-manager)
8. [Configure Nginx Reverse Proxy](#configure-nginx-reverse-proxy)
9. [Setup SSL with Let's Encrypt](#setup-ssl-with-lets-encrypt)
10. [Configure Webmin](#configure-webmin)
11. [Monitoring and Maintenance](#monitoring-and-maintenance)
12. [Troubleshooting](#troubleshooting)

---

## 🔧 Prerequisites

### What You Need:
- AlmaLinux 8/9 VPS with root access
- Webmin installed
- Domain name (optional but recommended)
- SSH access to your server

### Minimum Requirements:
- **CPU**: 2 cores (4+ recommended for video processing)
- **RAM**: 4GB (8GB+ recommended)
- **Storage**: 50GB+ (SSD recommended)
- **Bandwidth**: Unlimited or high limit

---

## 🖥️ Initial Server Setup

### Step 1: Connect to Your VPS
```bash
ssh root@your-server-ip
```

### Step 2: Update System
```bash
# Update all packages
dnf update -y

# Install essential tools
dnf install -y wget curl git vim nano htop unzip
```

### Step 3: Create Non-Root User (Security Best Practice)
```bash
# Create new user
adduser videocompress
usermod -aG wheel videocompress

# Set password
passwd videocompress

# Switch to new user
su - videocompress
```

### Step 4: Configure SSH (Optional but Recommended)
```bash
# Edit SSH config
sudo vim /etc/ssh/sshd_config

# Add/modify these lines:
Port 2222                    # Change default port
PermitRootLogin no          # Disable root login
PasswordAuthentication no   # Use key-based auth only

# Restart SSH
sudo systemctl restart sshd
```

---

## 📦 Install Node.js and Dependencies

### Step 1: Install Node.js 18.x (LTS)
```bash
# Add NodeSource repository
curl -fsSL https://rpm.nodesource.com/setup_18.x | sudo bash -

# Install Node.js
sudo dnf install -y nodejs

# Verify installation
node --version
npm --version
```

### Step 2: Install Development Tools
```bash
# Install build tools for native modules
sudo dnf groupinstall -y "Development Tools"
sudo dnf install -y python3 make gcc gcc-c++
```

### Step 3: Install Additional Dependencies
```bash
# Install additional packages
sudo dnf install -y epel-release
sudo dnf install -y yum-utils
```

---

## 🎬 Install FFmpeg

### Step 1: Install FFmpeg from RPM Fusion
```bash
# Enable RPM Fusion repositories
sudo dnf install -y https://download1.rpmfusion.org/free/el/rpmfusion-free-release-$(rpm -E %rhel).noarch.rpm
sudo dnf install -y https://download1.rpmfusion.org/nonfree/el/rpmfusion-nonfree-release-$(rpm -E %rhel).noarch.rpm

# Install FFmpeg
sudo dnf install -y ffmpeg ffmpeg-devel

# Verify installation
ffmpeg -version
```

### Step 2: Test FFmpeg Installation
```bash
# Test with a simple command
ffmpeg -f lavfi -i testsrc=duration=1:size=320x240:rate=1 -f null -
```

### Step 3: Install Additional Codecs (Optional)
```bash
# Install additional codecs for better compatibility
sudo dnf install -y x264 x265 libvpx
```

---

## 📁 Setup VideoCompress Application

### Step 1: Create Application Directory
```bash
# Create application directory
sudo mkdir -p /opt/videocompress
sudo chown videocompress:videocompress /opt/videocompress
cd /opt/videocompress
```

### Step 2: Download Application Files
```bash
# Clone or upload your application files
# Option 1: If you have the files locally, use SCP
# scp -r /path/to/your/node-js/* videocompress@your-server:/opt/videocompress/

# Option 2: Create files manually
sudo vim /opt/videocompress/package.json
```

### Step 3: Create package.json
```json
{
  "name": "videocompress-http",
  "version": "3.2.0-orientation",
  "description": "Video compression HTTP server with AI-powered speed selection",
  "main": "server.js",
  "scripts": {
    "start": "node server.js",
    "dev": "nodemon server.js"
  },
  "dependencies": {
    "express": "^4.18.2",
    "multer": "^1.4.5-lts.1",
    "fluent-ffmpeg": "^2.1.2",
    "form-data": "^4.0.0",
    "node-fetch": "^2.6.7"
  },
  "devDependencies": {
    "nodemon": "^3.0.1"
  },
  "keywords": ["video", "compression", "ffmpeg", "http", "server"],
  "author": "",
  "license": "MIT"
}
```

### Step 4: Install Dependencies
```bash
cd /opt/videocompress
npm install
```

### Step 5: Create Environment Configuration
```bash
# Create environment file
sudo vim /opt/videocompress/.env
```

Add the following content:
```env
# Server Configuration
PORT=8080
HOST=0.0.0.0

# File Upload Limits
MAX_UPLOAD_SIZE=2147483648

# Temporary Directory
TEMP_DIR=/tmp

# Logging
LOG_LEVEL=info

# Security
NODE_ENV=production
```

### Step 6: Test Application
```bash
# Test the application
cd /opt/videocompress
npm start

# In another terminal, test the health endpoint
curl http://localhost:8080/health
```

---

## 🔥 Configure Firewall

### Step 1: Configure Firewalld
```bash
# Start and enable firewalld
sudo systemctl start firewalld
sudo systemctl enable firewalld

# Allow SSH (if you changed the port, use that port)
sudo firewall-cmd --permanent --add-service=ssh

# Allow HTTP and HTTPS
sudo firewall-cmd --permanent --add-service=http
sudo firewall-cmd --permanent --add-service=https

# Allow custom port for VideoCompress (if needed)
sudo firewall-cmd --permanent --add-port=8080/tcp

# Reload firewall
sudo firewall-cmd --reload

# Check status
sudo firewall-cmd --list-all
```

### Step 2: Configure SELinux (if enabled)
```bash
# Check SELinux status
sestatus

# If SELinux is enabled, configure it
sudo setsebool -P httpd_can_network_connect 1
sudo setsebool -P httpd_can_network_relay 1
```

---

## ⚡ Setup PM2 Process Manager

### Step 1: Install PM2 Globally
```bash
# Install PM2
sudo npm install -g pm2

# Create PM2 configuration
sudo vim /opt/videocompress/ecosystem.config.js
```

### Step 2: Create PM2 Configuration
```javascript
module.exports = {
  apps: [{
    name: 'videocompress',
    script: 'server.js',
    cwd: '/opt/videocompress',
    instances: 1,
    autorestart: true,
    watch: false,
    max_memory_restart: '1G',
    env: {
      NODE_ENV: 'production',
      PORT: 8080
    },
    error_file: '/var/log/videocompress/err.log',
    out_file: '/var/log/videocompress/out.log',
    log_file: '/var/log/videocompress/combined.log',
    time: true
  }]
};
```

### Step 3: Create Log Directory
```bash
# Create log directory
sudo mkdir -p /var/log/videocompress
sudo chown videocompress:videocompress /var/log/videocompress
```

### Step 4: Start Application with PM2
```bash
cd /opt/videocompress
pm2 start ecosystem.config.js

# Save PM2 configuration
pm2 save

# Setup PM2 to start on boot
pm2 startup
sudo env PATH=$PATH:/usr/bin /usr/lib/node_modules/pm2/bin/pm2 startup systemd -u videocompress --hp /home/videocompress
```

### Step 5: PM2 Management Commands
```bash
# Check status
pm2 status

# View logs
pm2 logs videocompress

# Restart application
pm2 restart videocompress

# Stop application
pm2 stop videocompress

# Monitor resources
pm2 monit
```

---

## 🌐 Configure Nginx Reverse Proxy

### Step 1: Install Nginx
```bash
# Install Nginx
sudo dnf install -y nginx

# Start and enable Nginx
sudo systemctl start nginx
sudo systemctl enable nginx
```

### Step 2: Create Nginx Configuration
```bash
# Create site configuration
sudo vim /etc/nginx/conf.d/videocompress.conf
```

Add the following configuration:
```nginx
server {
    listen 80;
    server_name your-domain.com www.your-domain.com;
    
    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header Referrer-Policy "no-referrer-when-downgrade" always;
    add_header Content-Security-Policy "default-src 'self' http: https: data: blob: 'unsafe-inline'" always;
    
    # Client max body size for video uploads
    client_max_body_size 2G;
    
    # Proxy settings
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
        
        # Timeout settings for video processing
        proxy_connect_timeout 300s;
        proxy_send_timeout 300s;
        proxy_read_timeout 300s;
    }
    
    # Static files (if any)
    location /static/ {
        alias /opt/videocompress/public/;
        expires 1y;
        add_header Cache-Control "public, immutable";
    }
    
    # Health check endpoint
    location /health {
        proxy_pass http://127.0.0.1:8080/health;
        access_log off;
    }
}
```

### Step 3: Test and Reload Nginx
```bash
# Test configuration
sudo nginx -t

# Reload Nginx
sudo systemctl reload nginx

# Check status
sudo systemctl status nginx
```

---

## 🔒 Setup SSL with Let's Encrypt

### Step 1: Install Certbot
```bash
# Install EPEL repository (if not already installed)
sudo dnf install -y epel-release

# Install Certbot
sudo dnf install -y certbot python3-certbot-nginx
```

### Step 2: Obtain SSL Certificate
```bash
# Get SSL certificate
sudo certbot --nginx -d your-domain.com -d www.your-domain.com

# Test automatic renewal
sudo certbot renew --dry-run
```

### Step 3: Setup Automatic Renewal
```bash
# Add to crontab for automatic renewal
sudo crontab -e

# Add this line:
0 12 * * * /usr/bin/certbot renew --quiet
```

---

## 🖥️ Configure Webmin

### Step 1: Access Webmin
1. Open your browser and go to: `https://your-server-ip:10000`
2. Login with root credentials
3. Accept the SSL certificate warning

### Step 2: Configure System Settings
1. **System → System Information**: Monitor server resources
2. **System → Bootup and Shutdown**: Ensure services start on boot
3. **System → Users and Groups**: Manage user accounts

### Step 3: Configure Services in Webmin
1. **Servers → Nginx**: Manage Nginx configuration
2. **Servers → SSH Server**: Configure SSH settings
3. **System → Scheduled Cron Jobs**: Manage cron jobs

### Step 4: Monitor Application
1. **System → Running Processes**: Monitor Node.js processes
2. **System → System Logs**: View application logs
3. **System → Disk Usage**: Monitor disk space

### Step 5: Create Webmin Module for VideoCompress (Optional)
```bash
# Create custom Webmin module
sudo mkdir -p /usr/share/webmin/videocompress
sudo vim /usr/share/webmin/videocompress/module.info
```

Add module information:
```ini
name=VideoCompress
desc=Video Compression Server Management
version=1.0
depends=proc
```

---

## 📊 Monitoring and Maintenance

### Step 1: Setup Log Rotation
```bash
# Create logrotate configuration
sudo vim /etc/logrotate.d/videocompress
```

Add configuration:
```
/var/log/videocompress/*.log {
    daily
    missingok
    rotate 30
    compress
    delaycompress
    notifempty
    create 644 videocompress videocompress
    postrotate
        pm2 reloadLogs
    endscript
}
```

### Step 2: Setup Monitoring Scripts
```bash
# Create monitoring script
sudo vim /opt/videocompress/monitor.sh
```

Add monitoring script:
```bash
#!/bin/bash

# Check if application is running
if ! pm2 list | grep -q "videocompress.*online"; then
    echo "$(date): VideoCompress is down, restarting..." >> /var/log/videocompress/monitor.log
    pm2 restart videocompress
fi

# Check disk space
DISK_USAGE=$(df / | awk 'NR==2 {print $5}' | sed 's/%//')
if [ $DISK_USAGE -gt 90 ]; then
    echo "$(date): Disk usage is high: ${DISK_USAGE}%" >> /var/log/videocompress/monitor.log
fi

# Check memory usage
MEMORY_USAGE=$(free | awk 'NR==2{printf "%.2f", $3*100/$2}')
if (( $(echo "$MEMORY_USAGE > 90" | bc -l) )); then
    echo "$(date): Memory usage is high: ${MEMORY_USAGE}%" >> /var/log/videocompress/monitor.log
fi
```

Make it executable:
```bash
sudo chmod +x /opt/videocompress/monitor.sh
```

### Step 3: Setup Cron Jobs
```bash
# Add monitoring to crontab
sudo crontab -e

# Add these lines:
*/5 * * * * /opt/videocompress/monitor.sh
0 2 * * * find /tmp -name "*.mp4" -mtime +1 -delete
```

### Step 4: Backup Configuration
```bash
# Create backup script
sudo vim /opt/videocompress/backup.sh
```

Add backup script:
```bash
#!/bin/bash

BACKUP_DIR="/backup/videocompress"
DATE=$(date +%Y%m%d_%H%M%S)

# Create backup directory
mkdir -p $BACKUP_DIR

# Backup configuration files
tar -czf $BACKUP_DIR/config_$DATE.tar.gz \
    /opt/videocompress/package.json \
    /opt/videocompress/ecosystem.config.js \
    /opt/videocompress/.env \
    /etc/nginx/conf.d/videocompress.conf

# Backup PM2 configuration
pm2 save
cp ~/.pm2/dump.pm2 $BACKUP_DIR/pm2_dump_$DATE.pm2

# Clean old backups (keep last 7 days)
find $BACKUP_DIR -name "*.tar.gz" -mtime +7 -delete
find $BACKUP_DIR -name "*.pm2" -mtime +7 -delete
```

Make it executable:
```bash
sudo chmod +x /opt/videocompress/backup.sh
```

---

## 🔧 Troubleshooting

### Common Issues and Solutions

#### 1. Application Won't Start
```bash
# Check logs
pm2 logs videocompress

# Check if port is in use
sudo netstat -tlnp | grep :8080

# Check Node.js version
node --version

# Check dependencies
cd /opt/videocompress
npm list
```

#### 2. FFmpeg Not Found
```bash
# Check FFmpeg installation
which ffmpeg
ffmpeg -version

# Reinstall if needed
sudo dnf reinstall ffmpeg
```

#### 3. Permission Issues
```bash
# Fix ownership
sudo chown -R videocompress:videocompress /opt/videocompress
sudo chown -R videocompress:videocompress /var/log/videocompress

# Fix permissions
sudo chmod -R 755 /opt/videocompress
```

#### 4. Nginx Issues
```bash
# Check Nginx configuration
sudo nginx -t

# Check Nginx logs
sudo tail -f /var/log/nginx/error.log
sudo tail -f /var/log/nginx/access.log

# Restart Nginx
sudo systemctl restart nginx
```

#### 5. Firewall Issues
```bash
# Check firewall status
sudo firewall-cmd --list-all

# Check if port is open
sudo netstat -tlnp | grep :80
sudo netstat -tlnp | grep :443
```

#### 6. SSL Certificate Issues
```bash
# Check certificate status
sudo certbot certificates

# Renew certificate manually
sudo certbot renew

# Check certificate expiration
openssl x509 -in /etc/letsencrypt/live/your-domain.com/cert.pem -text -noout | grep "Not After"
```

### Performance Optimization

#### 1. Optimize Node.js
```bash
# Add to ecosystem.config.js
env: {
  NODE_ENV: 'production',
  PORT: 8080,
  UV_THREADPOOL_SIZE: 128
}
```

#### 2. Optimize Nginx
```bash
# Add to nginx.conf
worker_processes auto;
worker_connections 1024;
keepalive_timeout 65;
gzip on;
gzip_types text/plain text/css application/json application/javascript;
```

#### 3. Monitor Resources
```bash
# Install monitoring tools
sudo dnf install -y htop iotop iftop

# Monitor in real-time
htop
iotop
iftop
```

---

## 📋 Final Checklist

### ✅ Pre-Deployment
- [ ] Server updated and secured
- [ ] Node.js 18.x installed
- [ ] FFmpeg installed and tested
- [ ] Application files uploaded
- [ ] Dependencies installed
- [ ] Environment configured

### ✅ Deployment
- [ ] PM2 configured and running
- [ ] Nginx reverse proxy configured
- [ ] SSL certificate installed
- [ ] Firewall configured
- [ ] Webmin configured

### ✅ Post-Deployment
- [ ] Application accessible via domain
- [ ] SSL working correctly
- [ ] File uploads working
- [ ] Video compression working
- [ ] Monitoring scripts running
- [ ] Backup system configured

### ✅ Security
- [ ] Non-root user created
- [ ] SSH secured
- [ ] Firewall configured
- [ ] SSL certificate installed
- [ ] Regular updates scheduled

---

## 🚀 Quick Commands Reference

### Application Management
```bash
# Start application
pm2 start videocompress

# Stop application
pm2 stop videocompress

# Restart application
pm2 restart videocompress

# View logs
pm2 logs videocompress

# Monitor resources
pm2 monit
```

### System Management
```bash
# Check system status
systemctl status nginx
systemctl status firewalld

# View logs
journalctl -u nginx -f
journalctl -u firewalld -f

# Check disk usage
df -h

# Check memory usage
free -h
```

### Maintenance
```bash
# Update system
sudo dnf update -y

# Update Node.js packages
cd /opt/videocompress && npm update

# Backup configuration
/opt/videocompress/backup.sh

# Monitor application
/opt/videocompress/monitor.sh
```

---

## 📞 Support

If you encounter any issues:

1. **Check logs**: `pm2 logs videocompress`
2. **Check system status**: `systemctl status nginx`
3. **Verify configuration**: `nginx -t`
4. **Check firewall**: `firewall-cmd --list-all`
5. **Monitor resources**: `htop`

### Useful Commands for Debugging:
```bash
# Check if application is running
pm2 list

# Check port usage
sudo netstat -tlnp | grep :8080

# Check Nginx configuration
sudo nginx -t

# Check SSL certificate
sudo certbot certificates

# Check disk space
df -h

# Check memory usage
free -h
```

---

**🎉 Congratulations! Your VideoCompress server is now running on AlmaLinux with Webmin!**

Your application should be accessible at:
- **HTTP**: `http://your-domain.com`
- **HTTPS**: `https://your-domain.com`
- **API Docs**: `https://your-domain.com/api-docs`
- **Webmin**: `https://your-server-ip:10000`
