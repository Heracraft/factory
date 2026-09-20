# Coolify application type "Dockerfile" (08-dashboard.md 5.6): build from
# the monorepo root (`docker build -f apps/web/Dockerfile .`), health check
# path /healthz, no host port mapping so Traefik routes 3000 and deploys
# roll.

FROM node:24-alpine AS base
RUN corepack enable

FROM base AS build
WORKDIR /repo
COPY . .
RUN pnpm install --frozen-lockfile

# Build-time env: everything apps/web reads is PUBLIC_* (5.1) and read at
# runtime via $env/dynamic/public, so these ARGs only set the image's
# defaults — Coolify can still override them as plain container env vars
# without a rebuild.
ARG PUBLIC_API_URL
ARG PUBLIC_LOGTO_ENDPOINT
ARG PUBLIC_LOGTO_APP_ID
ARG PUBLIC_STRIPE_PUBLISHABLE_KEY
ENV PUBLIC_API_URL=$PUBLIC_API_URL \
	PUBLIC_LOGTO_ENDPOINT=$PUBLIC_LOGTO_ENDPOINT \
	PUBLIC_LOGTO_APP_ID=$PUBLIC_LOGTO_APP_ID \
	PUBLIC_STRIPE_PUBLISHABLE_KEY=$PUBLIC_STRIPE_PUBLISHABLE_KEY

RUN pnpm --filter web build

# Debug symbols the built app never uses; stripping brings the node binary
# from 131 MB to 114 MB, most of the difference between over and under the
# 200 MB checklist line below.
FROM base AS strip-node
RUN apk add --no-cache binutils \
 && strip -s /usr/local/bin/node -o /usr/local/bin/node-stripped

# adapter-node's build/ is a fully self-contained Rollup bundle: every
# import in it is a `node:*` builtin (verified with `grep -rhn "^import
# .* from '[^./]" build/**/*.js` before writing this stage), so the
# runtime image needs no node_modules and, short of the node binary
# itself, no npm/yarn/corepack either — those are most of node:24-alpine's
# own weight. Plain alpine plus that one binary keeps the image under the
# 200 MB checklist line; busybox (already in alpine) supplies wget,
# addgroup and adduser.
FROM alpine:3.22 AS runtime
RUN apk add --no-cache libstdc++ libgcc
COPY --from=strip-node /usr/local/bin/node-stripped /usr/local/bin/node
WORKDIR /app
ENV NODE_ENV=production PORT=3000 HOST=0.0.0.0
COPY --from=build /repo/apps/web/build ./build

RUN addgroup -S repose && adduser -S repose -G repose
USER repose

EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
	CMD wget -qO- http://127.0.0.1:3000/healthz || exit 1

CMD ["node", "build"]
