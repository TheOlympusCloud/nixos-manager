# nixmgr:host - what one NixOS machine (NIXMGR_HOST) configures, for the fleet views.
# Repo files are reported relative to the flake root; anything else is "nixos".
let
  flake = builtins.getFlake (builtins.getEnv "NIXMGR_FLAKE");
  name = builtins.getEnv "NIXMGR_HOST";
  machine = flake.nixosConfigurations.${name};
  inherit (machine) config options;
  lib = machine.pkgs.lib;

  root = "${toString flake.outPath}/";
  rel = f: let s = toString f; in if lib.hasPrefix root s then lib.removePrefix root s else "nixos";
  try = v: default: let r = builtins.tryEval v; in if r.success then r.value else default;
  text = d: if builtins.isString d then d else if builtins.isAttrs d then d.text or "" else "";
  # mkBefore/mkAfter leave an order wrapper around list definitions.
  unwrap = v: if builtins.isAttrs v && (v._type or "") == "order" then v.content else v;
  pkgName = p: try (p.pname or (builtins.parseDrvName (p.name or "unknown")).name) "unknown";
  defs = opt: try (opt.definitionsWithLocations or [ ]) [ ];
  defFiles = opt: lib.unique (map (d: rel d.file) (defs opt));

  packageDefs = lib.concatMap (
    d: map (p: {
      name = pkgName p;
      version = try (toString (p.version or "")) "";
      file = rel d.file;
    }) (lib.toList (unwrap d.value))
  ) (defs options.environment.systemPackages);
  packages = lib.mapAttrsToList (n: ds: {
    name = n;
    version = (builtins.head ds).version;
    files = lib.unique (map (d: d.file) ds);
  }) (lib.groupBy (d: d.name) packageDefs);

  enabledIn =
    group:
    lib.concatLists (
      lib.mapAttrsToList (
        n: o:
        let
          enable = if builtins.isAttrs o then o.enable or null else null;
        in
        if enable != null && lib.isOption enable && try (config.${group}.${n}.enable or false) false == true then
          [
            {
              inherit group;
              name = n;
              files = defFiles enable;
              description = try (text (enable.description or "")) "";
            }
          ]
        else
          [ ]
      ) (try options.${group} { })
    );
  features = lib.concatMap enabledIn [
    "services"
    "programs"
    "virtualisation"
    "hardware"
  ];

  hm = if config ? home-manager then try (config.home-manager.users or { }) { } else { };
  enabledPrograms = ps: builtins.attrNames (lib.filterAttrs (_: p: builtins.isAttrs p && try (p.enable or false) false == true) ps);
  users = lib.mapAttrsToList (n: u: {
    name = n;
    description = u.description or "";
    groups = u.extraGroups or [ ];
    home =
      if hm ? ${n} then
        {
          packages = lib.unique (map pkgName (try hm.${n}.home.packages [ ]));
          programs = enabledPrograms (try hm.${n}.programs { });
        }
      else
        null;
  }) (lib.filterAttrs (_: u: u.isNormalUser or false) (try config.users.users { }));

  managed = try (config.nixosManager.managedFile or null) null;
  files = lib.filter (f: f != "nixos") (
    lib.unique (
      lib.concatMap (p: p.files) packages
      ++ lib.concatMap (f: f.files) features
      ++ lib.concatMap defFiles [
        options.networking.hostName
        options.system.stateVersion
        options.users.users
        options.fileSystems
        options.time.timeZone
        options.networking.firewall.allowedTCPPorts
        options.boot.loader.grub.enable
        options.boot.loader.systemd-boot.enable
      ]
    )
  );
in
{
  inherit
    name
    packages
    features
    users
    files
    ;
  hostName = try config.networking.hostName "";
  system = machine.pkgs.stdenv.hostPlatform.system;
  release = try config.system.nixos.release "";
  stateVersion = try config.system.stateVersion "";
  timeZone = try config.time.timeZone null;
  kernel = try config.boot.kernelPackages.kernel.version null;
  managedFile = if managed == null then null else rel managed;
  ports = {
    tcp = try config.networking.firewall.allowedTCPPorts [ ];
    udp = try config.networking.firewall.allowedUDPPorts [ ];
    firewall = try config.networking.firewall.enable false;
  };
  fileSystems = lib.mapAttrsToList (mountPoint: fs: {
    inherit mountPoint;
    fsType = fs.fsType or "";
    device = toString (fs.device or "");
  }) (try config.fileSystems { });
}
