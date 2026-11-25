# A NixOS VM whose managed.json (configs/vm) gives alice two app configs, one with
# a secret, a third the app breaks, and a keyring entry, with real sops-nix and
# home-manager. A fake app rewrites the files between activations; the test checks
# that the declared keys come back and the app's stay, the secret's file mode,
# that a second activation leaves the files alone, that the broken file stays as
# it is, that GNOME Keyring gets the entry, and that neither secret is anywhere
# in the system's closure. Run by TestManagedConfigsInVMWithRealNix (KVM needed).
let
  nixpkgs = builtins.getFlake "github:NixOS/nixpkgs/nixos-26.05";
  sops-nix = builtins.getFlake "github:Mic92/sops-nix";
  home-manager = builtins.getFlake "github:nix-community/home-manager/release-26.05";
  pkgs = nixpkgs.legacyPackages.x86_64-linux;
  inherit (pkgs) lib;
  token = "nixmgr-token-5b07c2e1";
  pw = "nixmgr-keyring-pw-9e4a1d33";
  # The secrets, encrypted for a test age key, as nixos-manager encrypts them for a machine.
  secrets =
    pkgs.runCommand "nixmgr-test-secrets"
      {
        nativeBuildInputs = [
          pkgs.age
          pkgs.sops
        ];
      }
      ''
        export HOME=$TMPDIR
        mkdir $out
        age-keygen -o $out/key.txt
        printf '{"token": "%s", "pw": "%s"}' ${token} ${pw} > plain.json
        sops --encrypt --age "$(age-keygen -y $out/key.txt)" --input-type json --output-type json plain.json > $out/secrets.json
      '';
in
pkgs.testers.runNixOSTest {
  name = "nixos-manager-app-configs";
  # managed.nix defines nixpkgs.overlays, which a read-only pkgs refuses.
  node.pkgsReadOnly = false;
  nodes.machine = {
    imports = [
      (import ../fleet/modules/managed.nix ./configs/vm/managed.json)
      sops-nix.nixosModules.sops
      home-manager.nixosModules.home-manager
    ];
    users.users.alice = {
      isNormalUser = true;
      uid = 1000;
    };
    home-manager.useGlobalPkgs = true;
    # sops-nix wants its key outside the store, as a machine's is.
    sops.age.keyFile = "/var/lib/sops-nix/key.txt";
    system.activationScripts.nixmgrTestKey = "install -D -m 600 ${secrets}/key.txt /var/lib/sops-nix/key.txt";
    system.activationScripts.setupSecrets.deps = [ "nixmgrTestKey" ];
    sops.age.sshKeyPaths = [ ];
    sops.gnupg.sshKeyPaths = [ ];
    sops.secrets = lib.genAttrs [ "token" "pw" ] (_: {
      sopsFile = "${secrets}/secrets.json";
      format = "json";
    });
    services.gnome.gnome-keyring.enable = true;
    environment.systemPackages = [ pkgs.libsecret ];
  };
  testScript =
    { nodes, ... }:
    let
      closure = pkgs.closureInfo { rootPaths = [ nodes.machine.system.build.toplevel ]; };
    in
    ''
      import base64, json, os, shlex, tomllib

      def alice(cmd):
          env = "export XDG_RUNTIME_DIR=/run/user/1000 DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus; "
          return machine.succeed("su - alice -c " + shlex.quote(env + cmd))

      fake = "/home/alice/.config/fake/"
      def app_writes(name, text):
          alice(f"echo {base64.b64encode(text.encode()).decode()} | base64 -d > {fake}{name}")
      def activate():
          machine.succeed("systemctl restart home-manager-alice.service")

      # 2. Neither secret is in the system's closure: the writer reads them on the machine.
      with open("${closure}/store-paths") as f:
          paths = f.read().split()
      found = []
      for p in paths:
          files = [p] if os.path.isfile(p) else [os.path.join(d, n) for d, _, ns in os.walk(p) for n in ns]
          for fp in files:
              if os.path.islink(fp) or not os.path.isfile(fp):
                  continue
              with open(fp, "rb") as fh:
                  data = fh.read()
              if b"${token}" in data or b"${pw}" in data:
                  found.append(fp)
      assert not found, f"a secret is in the closure: {found}"

      start_all()
      machine.wait_for_unit("multi-user.target")
      machine.wait_for_unit("home-manager-alice.service")

      with subtest("the first activation creates the files, the one with a secret 0600"):
          assert json.loads(machine.succeed(f"cat {fake}settings.json")) == {"theme": "light", "server": {"url": "https://music.example", "token": "${token}"}}
          assert machine.succeed(f"stat -c '%a %U' {fake}settings.json").split() == ["600", "alice"]
          assert machine.succeed(f"stat -c %a {fake}config.toml").strip() == "644"

      with subtest("1. a fake app rewrites them; activation puts the declared keys back and keeps the app's"):
          app_writes("settings.json", json.dumps({"theme": "dark", "own": 1, "server": {"url": "http://changed", "app": True}}))
          app_writes("config.toml", "\n".join([
              "# the app", "[Application]", "EnableSystemTray = false", "WindowWidth = 900", "",
              "[[Servers]]", 'ID = "a1"', 'Hostname = "http://old"', 'Username = "alice"', "",
              "[[Servers]]", 'ID = "b2"', 'Hostname = "http://other"', "",
          ]))
          app_writes("broken.json", "{ not json")
          activate()
          assert json.loads(machine.succeed(f"cat {fake}settings.json")) == {"theme": "light", "own": 1, "server": {"url": "https://music.example", "app": True, "token": "${token}"}}
          toml = machine.succeed(f"cat {fake}config.toml")
          assert "# the app" in toml, toml
          assert tomllib.loads(toml) == {
              "Application": {"EnableSystemTray": True, "WindowWidth": 900},
              "Servers": [{"ID": "a1", "Hostname": "https://music.example", "Username": "alice"}, {"ID": "b2", "Hostname": "http://other"}],
          }, toml
          assert machine.succeed(f"stat -c %a {fake}settings.json").strip() == "600"

      with subtest("4. a file that does not parse stays as it is, and the log says so"):
          assert machine.succeed(f"cat {fake}broken.json") == "{ not json"
          machine.succeed("journalctl -u home-manager-alice.service | grep -F '~/.config/fake/broken.json: left as it is: it is not valid JSON'")
          machine.fail("journalctl -b | grep -F -e '${token}' -e '${pw}'")

      with subtest("3. a second activation does not touch the files"):
          before = machine.succeed(f"stat -c '%i %Y' {fake}settings.json {fake}config.toml")
          machine.sleep(1)
          activate()
          assert machine.succeed(f"stat -c '%i %Y' {fake}settings.json {fake}config.toml") == before

      with subtest("5. the keyring entry is stored in GNOME Keyring's login collection, with its attributes"):
          machine.succeed("loginctl enable-linger alice")
          machine.wait_for_unit("user@1000.service")
          machine.wait_for_file("/run/user/1000/bus")
          alice("printf testpw | gnome-keyring-daemon --unlock --components=secrets")
          alice("systemctl --user start nixos-manager-keyring.service")
          assert alice("secret-tool lookup service supersonic username a1").strip() == "${pw}"
          found = alice("secret-tool search --all service supersonic 2>&1")
          assert "/org/freedesktop/secrets/collection/login/" in found, found
          assert "label = Password for 'a1' on 'supersonic'" in found, found
          assert "attribute.username = a1" in found and "attribute.service = supersonic" in found, found
          # Unchanged, it is not stored again.
          alice("systemctl --user restart nixos-manager-keyring.service")
          assert machine.succeed("journalctl --user-unit nixos-manager-keyring.service | grep -c ': stored'").strip() == "1"
          machine.fail("journalctl -b | grep -F -e '${token}' -e '${pw}'")
    '';
}
