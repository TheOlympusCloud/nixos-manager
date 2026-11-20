# Web editor + MCP server for a Nix flake repository, with Nix, nixd, nixfmt and sops inside.
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
# The tools live in their own profile, separate from the image's nix-env profile. nixpkgs
# comes as a tarball: a github: reference asks GitHub's API, which limits anonymous callers.
# Everything runs as nixos-manager (uid 10001), a single-user Nix install of its own: it
# owns the store folder and Nix's state, so it can replace any program in the store, and
# nothing in this container may run as root. (Compose's data-owner, a container of its
# own, is the one root step.)
RUN printf 'experimental-features = nix-command flakes\n' >> /etc/nix/nix.conf \
 && nix build --profile /nix/var/nix/profiles/tools --impure --expr \
      'with import (builtins.getFlake "https://github.com/NixOS/nixpkgs/archive/refs/heads/nixos-26.05.tar.gz") { }; buildEnv { name = "tools"; paths = [ nixd nixfmt git openssh sops age ssh-to-age ]; }' \
 && nix-collect-garbage \
 && for f in passwd group; do cp --remove-destination "$(readlink -f /etc/$f)" /etc/$f; done \
 && echo 'nixos-manager:x:10001:10001:nixos-manager:/home/nixos-manager:/bin/sh' >> /etc/passwd \
 && echo 'nixos-manager:x:10001:' >> /etc/group \
 && mkdir -p -m 0700 /home/nixos-manager /var/lib/nixos-manager \
 && chown nixos-manager:nixos-manager /nix/store /home/nixos-manager /var/lib/nixos-manager \
 && chown -R nixos-manager:nixos-manager /nix/var
COPY --from=build /out/nixos-manager /usr/local/bin/nixos-manager
ENV PATH=/nix/var/nix/profiles/tools/bin:/nix/var/nix/profiles/default/bin:/usr/local/bin:/usr/bin:/bin \
    USER=nixos-manager \
    NIXMGR_LISTEN=0.0.0.0:8080 \
    NIXMGR_DATA=/var/lib/nixos-manager
USER 10001:10001
VOLUME /var/lib/nixos-manager
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/nixos-manager"]
