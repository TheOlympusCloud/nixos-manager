# nixmgr:host - what one NixOS machine (NIXMGR_HOST) configures, for the fleet views.
# Repo files are reported relative to the flake root; anything else is "nixos".
let
  flake = builtins.getFlake (builtins.getEnv "NIXMGR_FLAKE");
  name = builtins.getEnv "NIXMGR_HOST";
  machine = flake.nixosConfigurations.${name};
  inherit (machine) config options;
  lib = machine.pkgs.lib;

  root = "${toString flake.outPath}/";
  try = v: default: let r = builtins.tryEval v; in if r.success then r.value else default;
  # A machine imported as a folder (./hosts/<name>) is reported as the folder;
  # its definitions are in that folder's default.nix. Only absolute paths are
  # probed: pathExists fails uncatchably on names like "<unknown-file>".
  rel =
    f:
    let
      s0 = toString f;
      s = if lib.hasPrefix "/" s0 && builtins.pathExists "${s0}/default.nix" then "${s0}/default.nix" else s0;
    in
    if lib.hasPrefix root s then lib.removePrefix root s else "nixos";
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
        # Renamed or alias options (mkRenamedOptionModule) are invisible and mirror their target.
        if enable != null && lib.isOption enable && (enable.visible or true) != false
          && try (config.${group}.${n}.enable or false) false == true
        then
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
  # Inside try as a whole: reading a removed option (programs.octant) throws.
  enabledPrograms = ps: builtins.attrNames (lib.filterAttrs (_: p: try (builtins.isAttrs p && (p.enable or false) == true) false) ps);
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
  # attribute -> package name for managed.json packages (one bad entry does not hide the others)
  managedPackages = lib.mapAttrs (_: n: try n "") (try (config.nixosManager.packageNames or { }) { });
  # Each enabled systemd unit as switch-to-configuration compares it (nixos-26.05,
  # compare_units): a hash of the settings it restarts the unit for, a hash of
  # each setting it only reloads it for, and the settings that decide what it
  # does with a changed unit. [Install] and the [Unit] keys it ignores are left
  # out; keys are sorted stably, as their order does not matter but the order of
  # one key's values does. A disabled unit is masked: it counts as removed.
  hash = builtins.hashString "sha256";
  unitIgnored = map (k: "Unit/${k}") [
    "Description"
    "Documentation"
    "OnFailure"
    "OnSuccess"
    "OnFailureJobMode"
    "IgnoreOnIsolate"
    "StopWhenUnneeded"
    "RefuseManualStart"
    "RefuseManualStop"
    "AllowIsolate"
    "CollectMode"
    "SourcePath"
  ];
  reloadKeys = [
    "Unit/X-Reload-Triggers"
    "Service/ExecReload"
    "Mount/Options"
  ];
  flagKeys = [
    "Service/X-ReloadIfChanged"
    "Service/X-RestartIfChanged"
    "Unit/RefuseManualStop"
    "Unit/X-OnlyManualStart"
  ];
  # [ { key = "Section/Key"; value = "..."; } ] in file order, as NixOS writes
  # units: "[Section]" and "Key=value" lines. No escapes or classes in the
  # regex: Nix's regex engine differs between versions.
  unitEntries =
    text:
    (builtins.foldl' (
      acc: line:
      let
        kv = builtins.match "([^=]+)=(.*)" line;
      in
      if lib.hasPrefix "[" line && lib.hasSuffix "]" line then
        acc // { section = builtins.substring 1 (builtins.stringLength line - 2) line; }
      else if kv == null || lib.hasPrefix "#" line || lib.hasPrefix ";" line || acc.section == "Install" then
        acc
      else
        acc
        // {
          entries = acc.entries ++ [
            {
              key = "${acc.section}/${builtins.head kv}";
              value = builtins.elemAt kv 1;
            }
          ];
        }
    ) { section = ""; entries = [ ]; } (lib.splitString "\n" text)).entries;
  entriesText = es: lib.concatMapStrings (e: "${e.key}=${e.value}\n") (builtins.sort (a: b: a.key < b.key) es);
  unitInfo =
    u:
    let
      entries = unitEntries (if u.text or null == null then "" else u.text);
      with' = keys: lib.filter (e: builtins.elem e.key keys) entries;
    in
    {
      hash = hash (entriesText (lib.filter (e: !(builtins.elem e.key (unitIgnored ++ reloadKeys))) entries));
      reload = lib.genAttrs (lib.filter (k: with' [ k ] != [ ]) reloadKeys) (k: hash (entriesText (with' [ k ])));
      # listToAttrs keeps the first of a name: reversed, the last value wins.
      flags = builtins.listToAttrs (map (e: lib.nameValuePair e.key e.value) (lib.reverseList (with' flagKeys)));
    };
  units = lib.mapAttrs (_: unitInfo) (lib.filterAttrs (_: u: u.enable or true) config.systemd.units);
  # Each enabled /etc entry by target: a hash of the file it links to, its mode
  # and owner. "${source}" copies a path in the flake to the store by content,
  # as the etc builder does, so editing another file does not change it.
  # /etc/systemd/system is listed as units.
  etc = lib.mapAttrs' (
    _: e: lib.nameValuePair e.target (hash "${e.source}\n${e.mode}\n${e.user}\n${e.group}")
  ) (lib.filterAttrs (_: e: e.enable && e.target != "systemd/system") config.environment.etc);

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
  # The whole system: a machine changes exactly when this does. Not in try: a
  # system that does not evaluate (a failed assertion) cannot be deployed, and
  # the machine shows why.
  toplevel = config.system.build.toplevel.drvPath;
  # What only a reboot applies, as toplevel links it: the kernel and its modules
  # (a new nvidia driver needs a reboot too), the initrd and the kernel
  # parameters. A container boots no kernel of its own. "or null": a NixOS
  # release without one of them still shows the rest.
  boot =
    if config.boot.kernel.enable or (!config.boot.isContainer) then
      builtins.toJSON [
        config.boot.kernelPackages.kernel.drvPath
        (config.system.modulesTree.drvPath or null)
        (if config.boot.initrd.enable or true then config.system.build.initialRamdisk.drvPath or null else null)
        config.boot.kernelParams
      ]
    else
      "";
  systemPath = config.system.build.toplevel.outPath;
  # null when they cannot be read: the diff then lists no unit or /etc file.
  # deepSeq: tryEval alone would not catch an error inside them.
  units = try (builtins.deepSeq units units) null;
  etc = try (builtins.deepSeq etc etc) null;
  managedFile = if managed == null then null else rel managed;
  inherit managedPackages;
  checks = {
    ssh = try config.services.openssh.enable false;
    sshPasswordAuthentication = try config.services.openssh.settings.PasswordAuthentication null;
    sshPermitRootLogin = try config.services.openssh.settings.PermitRootLogin null;
    fail2ban = try config.services.fail2ban.enable false;
    autoGC = try config.nix.gc.automatic false;
    autoOptimise = try (config.nix.optimise.automatic || config.nix.settings.auto-optimise-store) false;
    zram = try config.zramSwap.enable false;
    comin = try config.services.comin.enable false;
    cominRemotes = try (map (r: r.url) config.services.comin.remotes) [ ];
    # The branches comin follows on each remote. deepSeq: tryEval alone would
    # not catch an error inside the list, and the machine would fail to show.
    cominBranches =
      let
        branches = map (r: {
          inherit (r) url;
          main = r.branches.main.name or "main";
          testing = r.branches.testing.name or "";
        }) config.services.comin.remotes;
      in
      try (builtins.deepSeq branches branches) [ ];
    # Keys that let a normal user in over SSH: without one, keys-only SSH locks everyone out.
    sshUserKeys = try (lib.length (
      lib.concatMap (u: u.openssh.authorizedKeys.keys ++ u.openssh.authorizedKeys.keyFiles) (
        lib.attrValues (lib.filterAttrs (_: u: u.isNormalUser) config.users.users)
      )
    )) null;
  };
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
