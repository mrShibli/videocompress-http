#!/bin/bash

# 🚀 VideoCompress VPS Setup Script for AlmaLinux
# This script automates the basic setup process

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   print_error "This script must be run as root"
   exit 1
fi

# Configuration variables
APP_USER="videocompress"
APP_DIR="/opt/videocompress"
LOG_DIR="/var/log/videocompress"
DOMAIN_NAME=""
SERVER_IP=""

# Get server IP
SERVER_IP=$(curl -s ifconfig.me)

print_status "Starting VideoCompress VPS Setup..."
print_status "Server IP: $SERVER_IP"

# Function to update system
update_system() {
    print_status "Updating system packages..."
    dnf update -y
    dnf install -y wget curl git vim nano htop unzip epel-release yum-utils
    print_success "System updated successfully"
}

# Function to create application user
create_user() {
    print_status "Creating application user: $APP_USER"
    
    if id "$APP_USER" &>/dev/null; then
        print_warning "User $APP_USER already exists"
    else
        adduser $APP_USER
        usermod -aG wheel $APP_USER
        print_success "User $APP_USER created successfully"
    fi
}

# Function to install Node.js
install_nodejs() {
    print_status "Installing Node.js 18.x..."
    
    # Add NodeSource repository
    curl -fsSL https://rpm.nodesource.com/setup_18.x | bash -
    
    # Install Node.js
    dnf install -y nodejs
    
    # Install development tools
    dnf groupinstall -y "Development Tools"
    dnf install -y python3 make gcc gcc-c++
    
    # Verify installation
    NODE_VERSION=$(node --version)
    NPM_VERSION=$(npm --version)
    print_success "Node.js $NODE_VERSION and npm $NPM_VERSION installed"
}

# Function to install FFmpeg
install_ffmpeg() {
    print_status "Installing FFmpeg..."
    
    # Enable RPM Fusion repositories
    dnf install -y https://download1.rpmfusion.org/free/el/rpmfusion-free-release-$(rpm -E %rhel).noarch.rpm
    dnf install -y https://download1.rpmfusion.org/nonfree/el/rpmfusion-nonfree-release-$(rpm -E %rhel).noarch.rpm
    
    # Install FFmpeg
    dnf install -y ffmpeg ffmpeg-devel x264 x265 libvpx
    
    # Test FFmpeg
    if command -v ffmpeg &> /dev/null; then
        FFMPEG_VERSION=$(ffmpeg -version | head -n1 | cut -d' ' -f3)
        print_success "FFmpeg $FFMPEG_VERSION installed successfully"
    else
        print_error "FFmpeg installation failed"
        exit 1
    fi
}

# Function to setup application directory
setup_app_directory() {
    print_status "Setting up application directory..."
    
    mkdir -p $APP_DIR
    mkdir -p $LOG_DIR
    chown $APP_USER:$APP_USER $APP_DIR
    chown $APP_USER:$APP_USER $LOG_DIR
    
    print_success "Application directory created: $APP_DIR"
}

# Function to install PM2
install_pm2() {
    print_status "Installing PM2 process manager..."
    
    npm install -g pm2
    
    print_success "PM2 installed successfully"
}

# Function to install Nginx
install_nginx() {
    print_status "Installing Nginx..."
    
    dnf install -y nginx
    systemctl start nginx
    systemctl enable nginx
    
    print_success "Nginx installed and started"
}

# Function to configure firewall
configure_firewall() {
    print_status "Configuring firewall..."
    
    systemctl start firewalld
    systemctl enable firewalld
    
    firewall-cmd --permanent --add-service=ssh
    firewall-cmd --permanent --add-service=http
    firewall-cmd --permanent --add-service=https
    firewall-cmd --permanent --add-port=8080/tcp
    
    firewall-cmd --reload
    
    print_success "Firewall configured"
}

# Function to create application files
create_app_files() {
    print_status "Creating application files..."
    
    # Create package.json
    cat > $APP_DIR/package.json << 'EOF'
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
EOF

    # Create PM2 ecosystem config
    cat > $APP_DIR/ecosystem.config.js << 'EOF'
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
EOF

    # Create environment file
    cat > $APP_DIR/.env << 'EOF'
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
EOF

    # Set ownership
    chown -R $APP_USER:$APP_USER $APP_DIR
    
    print_success "Application files created"
}

# Function to create Nginx configuration
create_nginx_config() {
    print_status "Creating Nginx configuration..."
    
    cat > /etc/nginx/conf.d/videocompress.conf << 'EOF'
server {
    listen 80;
    server_name _;
    
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
    
    # Health check endpoint
    location /health {
        proxy_pass http://127.0.0.1:8080/health;
        access_log off;
    }
}
EOF

    # Test and reload Nginx
    nginx -t
    systemctl reload nginx
    
    print_success "Nginx configuration created"
}

# Function to create monitoring script
create_monitoring_script() {
    print_status "Creating monitoring script..."
    
    cat > $APP_DIR/monitor.sh << 'EOF'
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
EOF

    chmod +x $APP_DIR/monitor.sh
    chown $APP_USER:$APP_USER $APP_DIR/monitor.sh
    
    print_success "Monitoring script created"
}

# Function to setup log rotation
setup_log_rotation() {
    print_status "Setting up log rotation..."
    
    cat > /etc/logrotate.d/videocompress << 'EOF'
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
EOF

    print_success "Log rotation configured"
}

# Function to display final instructions
display_final_instructions() {
    echo ""
    echo "🎉 VideoCompress VPS Setup Complete!"
    echo ""
    echo "📋 Next Steps:"
    echo "1. Upload your server.js file to: $APP_DIR/"
    echo "2. Install dependencies: cd $APP_DIR && npm install"
    echo "3. Start the application: pm2 start ecosystem.config.js"
    echo "4. Save PM2 configuration: pm2 save"
    echo "5. Setup PM2 startup: pm2 startup"
    echo ""
    echo "🌐 Access URLs:"
    echo "   - Application: http://$SERVER_IP"
    echo "   - API Docs: http://$SERVER_IP/api-docs"
    echo "   - Health Check: http://$SERVER_IP/health"
    echo ""
    echo "🔧 Management Commands:"
    echo "   - View logs: pm2 logs videocompress"
    echo "   - Monitor: pm2 monit"
    echo "   - Restart: pm2 restart videocompress"
    echo "   - Status: pm2 status"
    echo ""
    echo "📚 For detailed setup instructions, see: VPS_SETUP_GUIDE.md"
    echo ""
}

# Main execution
main() {
    print_status "Starting VideoCompress VPS setup..."
    
    update_system
    create_user
    install_nodejs
    install_ffmpeg
    setup_app_directory
    install_pm2
    install_nginx
    configure_firewall
    create_app_files
    create_nginx_config
    create_monitoring_script
    setup_log_rotation
    
    print_success "Basic setup completed successfully!"
    display_final_instructions
}

# Run main function
main "$@"
