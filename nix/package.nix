{
  lib,
  buildGoModule,
  buildNpmPackage,
  importNpmLock,
  makeWrapper,
  git,
  openssh,
  nixd,
  nixfmt,
  sops,
  age,
  ssh-to-age,
  version ? "dev",
}:

let
  # The web UI; the Go binary embeds web/dist/app (web/embed.go).
  ui = buildNpmPackage {
    pname = "nixos-manager-ui";
    inherit version;
    src = ../web;
    # Every dependency comes from the integrity hashes in package-lock.json: no npm hash to keep up to date.
    npmDeps = importNpmLock { npmRoot = ../web; };
    npmConfigHook = importNpmLock.npmConfigHook;
    installPhase = ''
      runHook preInstall
      cp -r dist/app $out
      runHook postInstall
    '';
  };
in
buildGoModule {
  pname = "nixos-manager";
  inherit version;
  src = ../.;

  # The NAR hash of `go mod vendor`. When go.mod, go.sum or the imported packages change, the build
  # fails with "hash mismatch" and prints the right value ("got: sha256-..."): put that here.
  vendorHash = "sha256-Mk/HzUnOJzMbBOMTynHxL1rl310DXmSPo43S8dxdW3k=";

  subPackages = [ "cmd/nixos-manager" ];
  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];

  # Not preBuild: the vendoring derivation runs that hook too, and must not depend on the UI.
  postConfigure = ''
    rm -rf web/dist/app
    cp -r ${ui} web/dist/app
  '';

  nativeBuildInputs = [ makeWrapper ];
  # nix itself is not here: it comes from the system (the NixOS module puts config.nix.package on PATH).
  postInstall = ''
    wrapProgram $out/bin/nixos-manager --prefix PATH : ${
      lib.makeBinPath [
        git
        openssh
        nixd
        nixfmt
        # The fleet's secrets (Settings → Secrets).
        sops
        age
        ssh-to-age
      ]
    }
  '';

  passthru = { inherit ui; };

  meta = {
    description = "Web console and MCP server for a fleet of NixOS machines kept in one flake repository";
    homepage = "https://github.com/TheOlympusCloud/nixos-manager";
    mainProgram = "nixos-manager";
    platforms = lib.platforms.linux;
  };
}
