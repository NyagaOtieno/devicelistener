# Use Node LTS (lightweight)
FROM node:20-alpine

# Create app directory
WORKDIR /app

# Install dependencies first (better caching)
COPY package*.json ./

RUN npm install --production

# Copy source code
COPY . .

# Expose your app port (change if different)
EXPOSE 3001

# Start app
CMD ["node", "server.js"]