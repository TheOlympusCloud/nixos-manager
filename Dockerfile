# Web editor + MCP server for a Nix flake repository, with Nix, nixd and nixfmt inside.
FROM node:24.22.3-trixie-slim AS ui
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26.11-trixie AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /src/web/dist/app ./web/dist/app
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/nixos-manager ./cmd/nixos-manager

FROM nixos/nix:2.35.6
# The tools live in their own profile, separate from the image's nix-env profile.
RUN printf 'experimental-features = nix-command flakes\n' >> /etc/nix/nix.conf \
 && nix build --profile /nix/var/nix/profiles/tools --impure --expr \
      'with import (builtins.getFlake "github:NixOS/nixpkgs/nixos-26.05") { }; buildEnv { name = "tools"; paths = [ nixd nixfmt git openssh ]; }' \
 && nix-collect-garbage
COPY --from=build /out/nixos-manager /usr/local/bin/nixos-manager
ENV PATH=/nix/var/nix/profiles/tools/bin:/root/.nix-profile/bin:/nix/var/nix/profiles/default/bin:/usr/local/bin:/usr/bin:/bin \
    NIXMGR_LISTEN=0.0.0.0:8080 \
    NIXMGR_DATA=/var/lib/nixos-manager
VOLUME /var/lib/nixos-manager
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/nixos-manager"]
