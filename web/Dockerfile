# syntax=docker/dockerfile:1

# ==========================================
# STAGE 1: Install dependencies
# ==========================================
FROM node:22-alpine AS deps

WORKDIR /app

# npm ci needs the lockfile — generate it with `npm install` in web/ and
# commit it (the CI web job enforces its presence).
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci

# ==========================================
# STAGE 2: Build (standalone output)
# ==========================================
FROM node:22-alpine AS builder

WORKDIR /app

COPY --from=deps /app/node_modules ./node_modules
COPY . .

ENV NEXT_TELEMETRY_DISABLED=1

# No AUTH_* / ORDER_API_URL needed here: all config is read at request
# time, so secrets stay on the container resource — never baked into the
# image (same rule as the Go service).
RUN npm run build

# ==========================================
# STAGE 3: Minimal runtime image
# ==========================================
FROM node:22-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000 \
    HOSTNAME=0.0.0.0

RUN addgroup -S app && adduser -S app -G app

# output: "standalone" emits a self-contained server.js + pruned deps.
COPY --from=builder --chown=app:app /app/.next/standalone ./
COPY --from=builder --chown=app:app /app/.next/static ./.next/static
COPY --from=builder --chown=app:app /app/public ./public

USER app

EXPOSE 3000

CMD ["node", "server.js"]
