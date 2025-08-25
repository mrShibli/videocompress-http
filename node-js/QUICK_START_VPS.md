# 🚀 Quick Start: VideoCompress VPS Deployment

## ⚡ 5-Minute Setup Guide

### Prerequisites
- AlmaLinux 8/9 VPS with root access
- SSH connection to your server
- Domain name (optional)

---

## 🎯 Step-by-Step Quick Setup

### 1. Connect to Your VPS
```bash
ssh root@your-server-ip
```

### 2. Download and Run Setup Script
```bash
# Download the setup script
wget https://raw.githubusercontent.com/your-repo/videocompress-http/main/node-js/setup-vps.sh

# Make it executable
chmod +x setup-vps.sh

# Run the setup script
./setup-vps.sh
```

### 3. Upload Your Application Files
```bash
# Create a temporary directory for uploads
mkdir -p /tmp/videocompress-upload

# Upload your files (from your local machine)
scp -r /path/to/your/node-js/* root@your-server-ip:/tmp/videocompress-upload/

# Move files to application directory
mv /tmp/videocompress-upload/* /opt/videocompress/
chown -R videocompress:videocompress /opt/videocompress
```

### 4. Install Dependencies and Start
```bash
# Switch to application user
su - videocompress

# Install dependencies
cd /opt/videocompress
npm install

# Start the application
pm2 start ecosystem.config.js

# Save PM2 configuration
pm2 save

# Setup PM2 to start on boot
pm2 startup
sudo env PATH=$PATH:/usr/bin /usr/lib/node_modules/pm2/bin/pm2 startup systemd -u videocompress --hp /home/videocompress
```

### 5. Test Your Application
```bash
# Test health endpoint
curl http://your-server-ip/health

# Test web interface
curl http://your-server-ip/
```

---

## 🌐 Access Your Application

Once setup is complete, your application will be available at:

- **Web Interface**: `http://your-server-ip/`
- **API Documentation**: `http://your-server-ip/api-docs`
- **Health Check**: `http://your-server-ip/health`

---

## 🔧 Essential Management Commands

### Application Management
```bash
# Check status
pm2 status

# View logs
pm2 logs videocompress

# Restart application
pm2 restart videocompress

# Monitor resources
pm2 monit
```

### System Management
```bash
# Check Nginx status
systemctl status nginx

# Check firewall
firewall-cmd --list-all

# Check disk usage
df -h

# Check memory usage
free -h
```

---

## 🔒 Security Setup (Recommended)

### 1. Setup SSL Certificate
```bash
# Install Certbot
dnf install -y certbot python3-certbot-nginx

# Get SSL certificate (replace with your domain)
certbot --nginx -d your-domain.com

# Setup automatic renewal
echo "0 12 * * * /usr/bin/certbot renew --quiet" | sudo crontab -
```

### 2. Secure SSH
```bash
# Edit SSH configuration
vim /etc/ssh/sshd_config

# Add/modify these lines:
Port 2222
PermitRootLogin no
PasswordAuthentication no

# Restart SSH
systemctl restart sshd
```

### 3. Configure Firewall
```bash
# Allow SSH on new port (if changed)
firewall-cmd --permanent --add-port=2222/tcp

# Remove old SSH port
firewall-cmd --permanent --remove-service=ssh

# Reload firewall
firewall-cmd --reload
```

---

## 📊 Monitoring Setup

### 1. Setup Monitoring Script
```bash
# The setup script already created this, but you can customize it
vim /opt/videocompress/monitor.sh
```

### 2. Add to Crontab
```bash
# Add monitoring to crontab
crontab -e

# Add this line:
*/5 * * * * /opt/videocompress/monitor.sh
```

---

## 🚨 Troubleshooting

### Common Issues

#### Application Won't Start
```bash
# Check logs
pm2 logs videocompress

# Check if port is in use
netstat -tlnp | grep :8080

# Check Node.js version
node --version
```

#### FFmpeg Issues
```bash
# Check FFmpeg installation
which ffmpeg
ffmpeg -version

# Reinstall if needed
dnf reinstall ffmpeg
```

#### Nginx Issues
```bash
# Check configuration
nginx -t

# Check logs
tail -f /var/log/nginx/error.log
```

#### Permission Issues
```bash
# Fix ownership
chown -R videocompress:videocompress /opt/videocompress
chown -R videocompress:videocompress /var/log/videocompress
```

---

## 📋 Post-Setup Checklist

- [ ] Application accessible via IP address
- [ ] Health endpoint responding
- [ ] File uploads working
- [ ] Video compression working
- [ ] PM2 process running
- [ ] Nginx serving correctly
- [ ] Firewall configured
- [ ] SSL certificate installed (if using domain)
- [ ] Monitoring script running
- [ ] Logs being generated

---

## 🆘 Need Help?

### Quick Diagnostics
```bash
# System status
systemctl status nginx
systemctl status firewalld
pm2 status

# Check logs
pm2 logs videocompress
tail -f /var/log/nginx/error.log

# Check resources
htop
df -h
free -h
```

### Support Resources
- **Full Setup Guide**: `VPS_SETUP_GUIDE.md`
- **API Documentation**: `http://your-server-ip/api-docs`
- **Application Logs**: `pm2 logs videocompress`
- **System Logs**: `journalctl -u nginx -f`

---

## 🎉 You're All Set!

Your VideoCompress server is now running on your VPS! 

**Next Steps:**
1. Test video compression with a sample file
2. Configure your domain name (if you have one)
3. Setup SSL certificate for HTTPS
4. Configure monitoring and alerts
5. Setup regular backups

**Access URLs:**
- **Web Interface**: `http://your-server-ip/`
- **API Docs**: `http://your-server-ip/api-docs`
- **Health Check**: `http://your-server-ip/health`

Happy video compressing! 🎬✨
