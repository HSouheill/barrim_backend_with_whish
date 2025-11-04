#!/bin/bash

# Development Setup Script for Barrim Backend

echo "🚀 Setting up Barrim Backend for Development"

# Create .env file if it doesn't exist
if [ ! -f .env ]; then
    echo "📝 Creating .env file..."
    cat > .env << EOF
# Development Environment Variables
ENV=development
PORT=8080
GIN_MODE=debug

# Database Configuration
MONGODB_URI=mongodb://admin:admin123@localhost:27017/barrim_dev?authSource=admin

# Redis Configuration
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
REDIS_DB=0

# JWT Configuration
JWT_SECRET=dev-jwt-secret-key-change-in-production

# Firebase Configuration (optional for development)
FIREBASE_PROJECT_ID=your-firebase-project-id
FIREBASE_PRIVATE_KEY_ID=your-private-key-id
FIREBASE_PRIVATE_KEY=your-private-key
FIREBASE_CLIENT_EMAIL=your-client-email
FIREBASE_CLIENT_ID=your-client-id
FIREBASE_AUTH_URI=https://accounts.google.com/o/oauth2/auth
FIREBASE_TOKEN_URI=https://oauth2.googleapis.com/token

# Twilio Configuration (optional for development)
TWILIO_ACCOUNT_SID=your-twilio-account-sid
TWILIO_AUTH_TOKEN=your-twilio-auth-token
TWILIO_PHONE_NUMBER=your-twilio-phone-number

# Email Configuration (optional for development)
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your-email@gmail.com
SMTP_PASSWORD=your-app-password

# File Upload Configuration
MAX_FILE_SIZE=10485760
ALLOWED_EXTENSIONS=jpg,jpeg,png,gif,pdf,doc,docx
EOF
    echo "✅ .env file created"
else
    echo "ℹ️  .env file already exists"
fi

# Create uploads directory if it doesn't exist
if [ ! -d uploads ]; then
    echo "📁 Creating uploads directory..."
    mkdir -p uploads/{bookings,category,certificates,companies,logo,logos,videos,profiles,serviceprovider,vouchers}
    echo "✅ Uploads directory created"
else
    echo "ℹ️  Uploads directory already exists"
fi

# Create tmp directory if it doesn't exist
if [ ! -d tmp ]; then
    echo "📁 Creating tmp directory..."
    mkdir -p tmp
    echo "✅ Tmp directory created"
else
    echo "ℹ️  Tmp directory already exists"
fi

echo "🎉 Development setup complete!"
echo ""
echo "To start the development environment:"
echo "  docker-compose -f docker-compose.dev.yml up --build"
echo ""
echo "To stop the development environment:"
echo "  docker-compose -f docker-compose.dev.yml down"
echo ""
echo "To view logs:"
echo "  docker-compose -f docker-compose.dev.yml logs -f backend"
