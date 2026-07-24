FROM node:22-alpine AS deps
WORKDIR /app
COPY package*.json ./
RUN npm ci --no-audit --no-fund

FROM deps AS development
WORKDIR /workspace
COPY . .
ENV NODE_ENV=development
EXPOSE 3000
CMD ["sh", "-c", "if [ ! -x node_modules/.bin/next ]; then cp -a /app/node_modules/. node_modules/; fi; exec npm run dev"]

FROM deps AS builder
COPY . .
ENV NEXT_OUTPUT=
ENV API_INTERNAL_URL=http://api:8080
RUN npm run build

FROM node:22-alpine AS production
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
COPY --from=deps /app/node_modules ./node_modules
COPY --from=builder /app/.next ./.next
COPY --from=builder /app/public ./public
COPY package*.json ./
EXPOSE 3000
CMD ["npm", "run", "start"]
