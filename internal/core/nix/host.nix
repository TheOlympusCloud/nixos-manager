# nixmgr:host - what one NixOS machine (host) configures, for the fleet views.
# Repo files are reported relative to the flake root; anything else is "nixos".
{ flakeRef, host }:
let
  flake = builtins.getFlake flakeRef;
  name = host;
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

  # What the system gets, by store path: a definition taken out by name
  # (managed.json's removedPackages) is not installed, while another of the same
  # name may be (nodejs_20 replaced by nodejs_24). null when it cannot be read.
  outPath = p: try (builtins.unsafeDiscardStringContext p.outPath) "";
  installed =
    let
      ps = map outPath config.environment.systemPackages;
    in
    try (builtins.deepSeq ps (lib.genAttrs ps (_: true))) null;
  packageDefs = lib.concatMap (
    d: map (p: {
      name = pkgName p;
      version = try (toString (p.version or "")) "";
      file = rel d.file;
      installed = installed == null || installed ? ${outPath p};
    }) (lib.toList (unwrap d.value))
  ) (defs options.environment.systemPackages);
  byName =
    ds:
    lib.mapAttrsToList (n: ds: {
      name = n;
      version = (builtins.head ds).version;
      files = lib.unique (map (d: d.file) ds);
    }) (lib.groupBy (d: d.name) ds);
  packages = byName (builtins.filter (d: d.installed) packageDefs);
  # Names with no copy installed; a name still installed is listed with what installs it.
  removedPackages =
    let
      have = lib.genAttrs (map (p: p.name) packages) (_: true);
    in
    builtins.filter (p: !(have ? ${p.name})) (byName (builtins.filter (d: !d.installed) packageDefs));

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
  homeFiles = n: lib.filterAttrs (_: f: f.enable or true) (try hm.${n}.home.file { });
  users = lib.mapAttrsToList (n: u: {
    name = n;
    description = u.description or "";
    groups = u.extraGroups or [ ];
    home =
      if hm ? ${n} then
        {
          packages = lib.unique (map pkgName (try hm.${n}.home.packages [ ]));
          programs = enabledPrograms (try hm.${n}.programs { });
          # What home-manager links into ~, relative to it; programs write some of them.
          files =
            let
              ts = lib.unique (map (f: f.target) (builtins.attrValues (homeFiles n)));
            in
            try (builtins.deepSeq ts ts) [ ];
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
  # does with a changed or removed unit. [Install] and the [Unit] keys it
  # ignores are left out; keys are sorted stably, as their order does not
  # matter but the order of one key's values does. A disabled unit is masked:
  # it counts as removed.
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
    "Unit/X-StopOnRemoval"
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
      # Only hashed: without the store paths' context, its keys can name attributes.
      entries = unitEntries (builtins.unsafeDiscardStringContext (if u.text or null == null then "" else u.text));
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
    _: e:
    lib.nameValuePair (builtins.unsafeDiscardStringContext e.target) (hash "${e.source}\n${e.mode}\n${e.user}\n${e.group}")
  ) (lib.filterAttrs (_: e: e.enable && e.target != "systemd/system") config.environment.etc);
  # Each home-manager user's files in ~ by target, hashed as /etc is.
  homeFileHashes = lib.mapAttrs (
    n: _:
    lib.mapAttrs' (
      _: f:
      lib.nameValuePair (builtins.unsafeDiscardStringContext f.target) (hash "${f.source}\n${toString (f.executable or "")}")
    ) (homeFiles n)
  ) hm;

  # Fleet modules (modules/fleet/<name>/managed.json, managed.nix 7): whether each
  # is on, where that is set, and each of its options' values and where they are set.
  fleetModules = lib.mapAttrsToList (n: opts: {
    name = n;
    enable = try (config.fleet.modules.${n}.enable == true) false;
    files = defFiles opts.enable;
    options = lib.mapAttrs (o: opt: {
      # null when it has no value: an option without a default that nothing sets.
      value =
        let
          v = config.fleet.modules.${n}.${o};
        in
        try (if lib.isDerivation v then null else builtins.deepSeq v v) null;
      files = defFiles opt;
    }) (lib.filterAttrs (o: opt: o != "enable" && lib.isOption opt) opts);
  }) (lib.filterAttrs (_: opts: builtins.isAttrs opts && lib.isOption (opts.enable or null)) (try (options.fleet.modules or { }) { }));

  files = lib.filter (f: f != "nixos") (
    lib.unique (
      map (d: d.file) packageDefs
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
    removedPackages
    features
    users
    files
    ;
  # NixOS's own packages for a working system (bash, coreutils…): removing one needs care.
  corePackages = try (map pkgName config.environment.corePackages) [ ];
  # Whether home-manager's NixOS module is loaded, so users can have a home configuration.
  homeManager = options ? home-manager;
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
  homeFiles = try (builtins.deepSeq homeFileHashes homeFileHashes) null;
  managedFile = if managed == null then null else rel managed;
  modules = fleetModules;
  # What its modules/managed.nix can apply: 1 before removed packages, overrides and home-manager.
  managedVersion = try (config.nixosManager.version or 1) 1;
  inherit managedPackages;
  # The sops-nix secrets it decrypts, never their values; null without sops-nix.
  secrets =
    if options ? sops then
      lib.mapAttrsToList (n: s: {
        name = n;
        path = try s.path "";
        file = try (rel s.sopsFile) "";
        owner = try (if s.owner == null then "" else s.owner) "";
        group = try (if s.group == null then "" else s.group) "";
        mode = try s.mode "";
      }) (try config.sops.secrets { })
    else
      null;
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
