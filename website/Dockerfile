# Stage 1: Base image.
## Start with a base image containing NodeJS so we can build Docusaurus.
FROM node:lts-alpine AS base
## Disable colour output from yarn to make logs easier to read.
ENV FORCE_COLOR=0
## Enable corepack.
RUN corepack enable
## Set the working directory to `/opt/docusaurus`.
WORKDIR /opt/docusaurus

# Stage 2: Production build.
FROM base AS prod
## Set the working directory to `/opt/docusaurus`.
WORKDIR /opt/docusaurus
## Copy over the source code.
COPY . /opt/docusaurus/
## Install dependencies with `--immutable` to ensure reproducibility.
RUN npm ci
## Build the static site.
RUN npm run build

# Stage 3: Serve static Docusaurus site and landing page with Caddy.
FROM caddy:2.10.0-alpine AS caddy
## Copy the Caddyfile.
COPY ./Caddyfile /etc/caddy/Caddyfile
## Copy the Docusaurus build output.
COPY --from=prod /opt/docusaurus/build /usr/share/caddy
# Copy the landing page assets.
COPY landing/images /usr/share/caddy/images
COPY landing/index.html landing/style.css /usr/share/caddy/
