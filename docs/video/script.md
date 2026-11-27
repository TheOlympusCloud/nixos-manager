# Nix, NixOS and nixos-manager: video script

A narrated tutorial in two parts. Part 1 explains how Nix and NixOS work. Part 2 walks through every section of nixos-manager: what each one is, why it exists, how to use it, with an example.

- **Source of truth:** `TheOlympusCloud/nixos-manager` at `main` 3df3c33 (PR #30, fleet modules). Every Part 2 claim was checked against the code (`web/src/views`, `web/src/components`, `internal/core`, `internal/mcpserver`, `nix/module.nix`, `templates/fleet`) and the README at that commit. If `main` moves before recording, re-check the scenes marked **[volatile]**.
- **Length:** about 7,100 words of narration, so about 47 minutes of speech at 150 words a minute. With pauses on diagrams and captures, expect 55 to 65 minutes: Part 1 about 20, Part 2 about 40. The per-scene durations are budgets that include those pauses; the audio length of each scene sets the final cut. Chapters are sized so the video can also ship as a series (one episode per chapter).
- **Voice:** one narrator, local TTS (Kokoro-82M, voice `af_heart` or `am_michael`, speed 1.0). No paid services.

## How to read this script

Each scene has:

- **Visual**: what is on screen. `[MANIM]` is an animation to build. `[CAPTURE]` is a screen recording of the console. `[CODE]` is a code card: a still or typed-on view of a file.
- **On screen**: short text overlays or lower-thirds, if any.
- **Narration**: the exact text for TTS, in the `>` blocks. It is written to be spoken: no symbols the voice would stumble on. Durations are budgets, narration plus pauses.

Pronunciation (apply as a lexicon or by respelling before TTS):

| Written | Say |
|---|---|
| NixOS | "nix O S" |
| nixpkgs | "nix packages" |
| comin | "co-min" |
| Forgejo | "for-jay-oh" |
| sops, sops-nix | "sops", "sops nix" |
| home-manager | "home manager" |
| `/nix/store` | "the nix store" |
| `managed.json` | "managed dot json" |
| `flake.lock` | "flake dot lock" |
| nixd | "nix D" |
| MCP | "M C P" |
| LXC | "L X C" |
| ISO | "I S O" (letters) |
| ntfy | "notify" |
| zram | "Z ram" |
| fail2ban | "fail to ban" |
| `mkForce` | "make force" |

## Visual language

- **Palette:** use the console's own Night theme (dark background, accent colour of the ❄ mark) so the Manim parts and the captures feel like one film. The console's Print theme (Settings → Theme) suits the daylight-looking diagrams if wanted.
- **Recurring icons:** a store path is a pill `/nix/store/<hash>-name`, with the hash in a dimmer colour. A machine is a rounded box with its name. A git commit is a dot on a line. A lock is a padlock. comin is a small arrow pulling from git into the machine.
- **Recurring fleet:** use the fixture fleet that ships with the repository for every capture, so names stay the same in animations and recordings: machines `homeserver`, `pi` and `workstation`, and a standalone home `pablo@laptop` (see Production notes).

---

## Chapter 0: Cold open (about 1:30)

### S0.1 Hook (0:45)

**Visual:** [MANIM] Three machines side by side: a home server, a Raspberry Pi, a desktop. Each has a messy pile of hand-edited config files and installed packages that slowly drift apart (colours change, files appear). Then a single git repository appears above them; the piles dissolve and each machine is rebuilt from the repository, identical where they should be identical.

**Narration:**
> You have a few Linux machines. A home server, a Raspberry Pi, a desktop. Over the years each one collects packages, edited config files, and fixes you no longer remember making. One day something breaks, and you can't tell what changed, or rebuild the machine the way it was.
>
> NixOS takes a different approach. The whole machine, every package, every service, every user, is described in files. Those files live in one git repository. And the machine is built from them, the same way, every time.

### S0.2 What this video covers (0:45)

**Visual:** [MANIM] A two-part chapter map. Part 1: store, derivations, flakes, modules, NixOS, generations, home-manager, secrets, deployment. Part 2: the nixos-manager console, with its navigation rail drawn as a list. Then a quick [CAPTURE] flyover of the console's fleet graph.

**Narration:**
> This video has two parts. In the first, we build up how Nix works from the ground: the Nix store, derivations, flakes, the module system, and how NixOS turns a description into a running machine you can roll back.
>
> In the second, we go through nixos-manager, the console that manages all of these machines from one place. Every view, every option, and what each one is for, with an example.

---

# Part 1: How Nix and NixOS work

## Chapter 1: The problem Nix solves (about 2:30)

### S1.1 Mutable systems drift (1:15)

**Visual:** [MANIM] A classic Linux file tree: `/usr/bin`, `/usr/lib`, `/etc`. Installing package A writes `libssl 3.0` into `/usr/lib`. Installing package B replaces it with `libssl 3.2`; package A turns red. A hand edit to `/etc/ssh/sshd_config` appears as a pencil mark that no record explains.

**On screen:** "One shared place, overwritten in place"

**Narration:**
> On a classic Linux system, every package installs into the same shared folders. Libraries go into slash user slash lib, programs into slash user slash bin, settings into slash etc. When two programs need different versions of the same library, the second install overwrites the first, and something breaks.
>
> Settings have the same problem. You edit a file in slash etc by hand, and nothing records why, or what it was before. Two machines that started identical slowly drift apart. This is called configuration drift, and it is the reason "it works on my machine" exists.

### S1.2 The Nix idea (1:15)

**Visual:** [MANIM] The shared `/usr/lib` box shatters. In its place, a row of separate sealed boxes, each labelled with a hash and a name: `libssl-3.0`, `libssl-3.2`, `nginx-1.26`. Package A points by arrow to `libssl-3.0`, package B to `libssl-3.2`. Both stay green.

**On screen:** "Every version in its own place. Nothing is overwritten."

**Narration:**
> Nix starts from one idea: never overwrite anything. Every package, in every version, goes into its own folder, whose name contains a fingerprint of exactly how it was built. Two versions of the same library simply sit next to each other. Each program points at precisely the one it was built with.
>
> Because nothing is overwritten, installing something new cannot break something old. And because each folder is described by how it was built, you can rebuild the same thing anywhere, and get the same result.

## Chapter 2: The Nix store (about 3:00)

### S2.1 Store paths (1:30)

**Visual:** [MANIM] Zoom into `/nix/store`. A list of paths scrolls:
`/nix/store/7x2kq…-openssl-3.0.15`, `/nix/store/b9mzc…-openssl-3.2.4`, `/nix/store/p1lww…-nginx-1.26.2`, `/nix/store/0dh4a…-bash-5.2`. Highlight the three parts of one path: the store prefix, the 32-character hash (dim), the name and version. A padlock icon appears on every path: read-only.

**On screen:** `/nix/store/<hash>-<name>-<version>`

**Narration:**
> Everything Nix builds lives in one folder: the Nix store, at slash nix slash store. Each entry is called a store path. Its name has three parts: the store itself, a hash, and a readable name with a version.
>
> The hash is the important part. It is computed from everything that went into the build: the source code, the compiler, the build script, the libraries, and their own hashes in turn. Change any input, even one compiler flag, and you get a different hash, so a different path. Nothing is ever changed in place: once a store path exists, it is read only.

### S2.2 Closures (1:30)

**Visual:** [MANIM] Start from `nginx`. Arrows fan out to `openssl`, `zlib`, `pcre2`, `glibc`; those point further down to `glibc`. The whole tree is outlined and labelled "closure of nginx". Then the outline is copied as one unit to another machine icon.

**On screen:** "Closure: a path plus everything it references"

**Narration:**
> A program in the store refers to its libraries by their full store paths. So nginx points to one exact openssl, one exact zlib, one exact C library. Follow those references all the way down and you get the closure: the program and everything it needs to run.
>
> The closure is self-contained. Copy it to another machine, and it runs there, because nothing depends on what happens to be installed outside it. This is also how binary caches work: instead of building a store path, Nix can download it from a cache that already built the same hash. Keep the word closure in mind. nixos-manager shows the closure of every machine later in this video.

## Chapter 3: Derivations and nixpkgs (about 3:30)

### S3.1 A derivation is a recipe (1:45)

**Visual:** [CODE] A minimal derivation, typed on:

```nix
stdenv.mkDerivation {
  pname = "hello";
  version = "2.12.1";
  src = fetchurl {
    url = "mirror://gnu/hello/hello-2.12.1.tar.gz";
    hash = "sha256-jZkUKv2SV28wsM18tCqNxoCZmLxdYH2Idh9RLibH2yA=";
  };
}
```

Then [MANIM] an arrow: "Nix expression" → "evaluate" → a `.drv` file in the store (`/nix/store/…-hello-2.12.1.drv`) → "build (realise)" → output path `/nix/store/…-hello-2.12.1`. A side branch from the `.drv`: "already in a binary cache?" → "download instead".

**Narration:**
> How does something get into the store? Through a derivation. A derivation is a build recipe: the source to fetch, with its hash, the build steps, and the other store paths it needs.
>
> Nix works in two steps. First it evaluates your Nix code, which produces the derivation, a small file in the store that ends in dot d r v. Nothing has been built yet. Then it realises the derivation: it runs the build in a sandbox, with no network and nothing visible except the declared inputs, and the result becomes the output store path.
>
> Because the output path is known before the build starts, Nix can first ask a binary cache whether someone already built it. Usually someone has, and the build becomes a download.

### S3.2 nixpkgs (1:45)

**Visual:** [MANIM] A huge tree labelled `nixpkgs` with branches `firefox`, `jellyfin`, `python3`, `linuxPackages`, `nixos/modules`… A counter ticks up to "over 120,000 packages". Then a branch label: `nixos-26.05` (stable) next to `nixos-unstable`.

**Narration:**
> Writing derivations for everything would be a lot of work. That is what nixpkgs is for: one large repository with derivations for well over a hundred thousand packages, and also all the NixOS modules we will meet soon.
>
> nixpkgs comes in branches. A stable release, like twenty six oh five, gets security fixes for about seven months. Unstable moves every day. A machine follows one of them, and you can still take a single package from another, which nixos-manager supports, as you will see.
>
> A word on the Nix language itself, since you will see it on screen. It is a small functional language. You will mostly see three things: attribute sets, which are curly braces holding name equals value pairs; functions, written as an argument, a colon, and a body; and let bindings, to name values. That is enough to read every file in this video.

## Chapter 4: Flakes (about 3:30)

### S4.1 What a flake is (1:45)

**Visual:** [CODE] The starter fleet's `flake.nix`, trimmed:

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
    home-manager.url = "github:nix-community/home-manager/release-26.05";
    comin.url = "github:nlewo/comin";
    sops-nix.url = "github:Mic92/sops-nix";
    disko.url = "github:nix-community/disko";
  };
  outputs = { nixpkgs, ... }@inputs: {
    nixosConfigurations.homeserver = nixpkgs.lib.nixosSystem { … };
  };
}
```

Highlight `inputs` in one colour and `outputs` in another. [MANIM] Inputs flow as arrows into a function box; outputs come out as named boxes: `nixosConfigurations.homeserver`, `nixosConfigurations.pi`.

**Narration:**
> A flake is a folder with a file called flake dot nix at its root. It has two halves. Inputs: the other flakes it depends on, like nixpkgs, home-manager, or comin. And outputs: what it provides, given those inputs. For a fleet of machines, the outputs that matter are the NixOS configurations, one per machine, each named after the machine.

### S4.2 The lock file and pure evaluation (1:45)

**Visual:** [CODE] A `flake.lock` excerpt with `"rev": "a3f1…"` and `"narHash": "sha256-…"`, then [MANIM] a padlock snapping onto each input arrow. Then a "pure evaluation" bubble around the flake: an arrow from inside to `/var/lib/…` bounces off with an error "forbidden in pure evaluation mode".

**On screen:** "Lock = the exact revision of every input"

**Narration:**
> Next to it sits flake dot lock. It pins every input to an exact git revision and a hash of its content. Whoever builds this flake, today or in a year, on any machine, gets exactly the same nixpkgs, and therefore the same packages. To move forward you update the lock on purpose, and you can review what changed.
>
> Flakes are also evaluated in pure mode. The code can't read random files from the computer it runs on, or environment variables, or the network outside the declared inputs. That is what makes the result reproducible. Keep this in mind: it is why, later, a new input always needs a Lock before anything evaluates.

## Chapter 5: The module system (about 4:00)

### S5.1 Options and config (1:30)

**Visual:** [MANIM] Two module boxes. Module A *declares* an option: `services.openssh.enable : bool, default false`. Module B *defines* it: `services.openssh.enable = true;`. The module system box in the middle merges them into one big `config` tree, from which `systemd.services.sshd` and `/etc/ssh/sshd_config` grow.

**Narration:**
> NixOS is built from modules. A module is a Nix file that can do two things: declare options, and set values for options.
>
> NixOS itself ships thousands of modules. One of them declares services dot openssh dot enable, a true or false option. In your own configuration, you set it to true. The module system collects every module, merges all the values, and the openssh module reacts: it creates the systemd service and writes the SSH config file. You describe what you want; the module works out how.

### S5.2 Merging and priorities (2:30)

**Visual:** [MANIM] Three merging rules, one after another:
1. Attribute sets merge key by key: two modules each set different keys of `services.openssh.settings`; they combine.
2. Lists concatenate: module A sets `allowedTCPPorts = [ 22 ]`, module B `[ 80 443 ]`, result `[ 22 80 443 ]`.
3. Two different single values conflict: `time.timeZone = "Europe/Madrid"` vs `"UTC"` → red error "conflicting definitions".

Then a vertical priority ladder (lower number wins), with labels on the right:

| Priority | Who |
|---|---|
| 50 | `lib.mkForce` |
| 90 | set on one machine in nixos-manager |
| 95 | set for every machine in nixos-manager |
| 100 | plain hand-written value |
| 1000 | `lib.mkDefault` |
| 1500 | option's own default |

**On screen:** "Lower number wins"

**Narration:**
> When several modules set the same option, the module system merges them. Attribute sets merge key by key. Lists are joined: if one module opens port twenty two and another opens eighty and four four three, the machine opens all three. A single value set twice to two different things is an error, and the build says which files disagree.
>
> To resolve that, every value has a priority, and the lower number wins. A plain value has priority one hundred. Make default is weaker, a thousand. Make force is stronger, fifty.
>
> Remember the two values in the middle of this ladder: ninety and ninety five. That is where nixos-manager writes what you set by clicking: ninety on one machine, ninety five for every machine. They beat what you wrote by hand, but never beat make force. One detail matters a lot later: at a winning priority, a list is not joined, it replaces the others whole.
>
> Finally, a module can switch things on conditionally, with make if. A module can declare its own enable option, and only apply its settings when that option is true. nixos-manager's fleet modules are built exactly that way.

## Chapter 6: From configuration to running system (about 3:30)

### S6.1 nixosSystem and the toplevel (1:30)

**Visual:** [MANIM] `nixosConfigurations.homeserver` → evaluate all modules → one derivation `system.build.toplevel` → its output store path `/nix/store/…-nixos-system-homeserver-26.05…`. Open that path like a folder: `kernel`, `initrd`, `etc/`, `sw/bin`, `activate`, `bin/switch-to-configuration`.

**Narration:**
> A NixOS configuration is evaluated into a single derivation called the system toplevel. Its output is one store path that contains the whole machine: the kernel, the initial ramdisk, every file that will appear in slash etc, every program on the path, and the script that activates it.
>
> If two configurations produce the same toplevel path, they are the same system. nixos-manager uses exactly this to decide whether a change affects a machine: if the toplevel changes, the machine changes.

### S6.2 Switching (2:00)

**Visual:** [MANIM] A timeline with the new toplevel arriving. `switch-to-configuration` with three modes drawn as three doors:
- **switch**: activate now and make it the boot default.
- **test**: activate now, boot entry untouched (a reboot returns to the old system).
- **boot**: only make it the boot default, for next reboot.

Then systemd units under the switch: `sshd.service` "restart", `nginx.service` "reload", `jellyfin.service` "start", `old.service` "stop". A kernel icon labelled "new kernel: reboot needed".

**Narration:**
> To apply a system, NixOS runs switch to configuration from the new toplevel. It has three modes. Switch activates the new system now and makes it the default at boot. Test activates it now but leaves the boot menu alone, so a reboot brings back the previous system. Boot does the opposite: nothing changes until the next reboot.
>
> Activating compares the old and new systemd units. Changed services are restarted or reloaded, new ones started, removed ones stopped. What cannot change while running, like a new kernel or initial ramdisk, waits for a reboot. nixos-manager shows you all of this before you deploy: which units restart, and whether a reboot is needed.

## Chapter 7: Generations, rollback and garbage collection (about 3:00)

### S7.1 Generations (1:30)

**Visual:** [MANIM] A stack of cards: generation 41, 42, 43, each pointing to its own toplevel path. A `/nix/var/nix/profiles/system` arrow points to 43. A boot menu mock-up lists all three. Then the arrow moves back to 42: "rollback".

**On screen:** `nixos-rebuild switch --rollback`

**Narration:**
> Each time you switch, NixOS records a new generation: a numbered pointer to that system's store path. The boot menu lists the recent generations. Since the old systems are still in the store, going back is instant: pick an older generation at boot, or run nixos rebuild switch with the rollback flag. Nothing needs to be rebuilt.

### S7.2 Garbage collection and stateVersion (1:30)

**Visual:** [MANIM] Store paths with arrows from "GC roots" (generations, profiles, result links). Unreferenced paths fade out when "nix-collect-garbage" runs. A slider "keep 14 days". Then a separate card: `system.stateVersion = "26.05";` with a padlock and "never change".

**Narration:**
> The store only grows, until you collect garbage. Anything still reachable from a root, like a generation you can boot into, is kept. Everything else is deleted. Usually you run it on a schedule and keep, say, fourteen days of generations, which is also how far back you can roll back. nixos-manager's Health view will remind you when a machine has no automatic collection.
>
> One option needs a warning: system state version. It is not the version the machine runs. It records the release the machine was first installed with, so that data formats of services like databases stay compatible. Set it once, and never change it.

## Chapter 8: home-manager (about 1:45)

### S8.1 Your user, declared (1:45)

**Visual:** [MANIM] The machine box splits into a system layer and a user layer for `pablo`. In the user layer: `programs.git`, `programs.zsh`, `~/.config/kitty/kitty.conf` as a link arrow into `/nix/store`. [CODE] `home-manager.users.pablo = import ../../home/pablo.nix;`.

**Narration:**
> NixOS describes the system. home-manager does the same for a user's home: their packages, their shell, and their dotfiles. A file it manages becomes a link into the Nix store, so it is versioned and identical on every machine.
>
> It runs in two ways. As a NixOS module, so the user's home is built and switched together with the system. Or standalone, for a machine that is not NixOS at all, like a Mac or another Linux. nixos-manager handles both.

## Chapter 9: Secrets with sops-nix (about 2:00)

### S9.1 Why the store is no place for secrets (2:00)

**Visual:** [MANIM] A password typed into a Nix string turns into a store path with an "everyone can read" eye icon. Red cross. Then the sops way: the value is encrypted into `secrets/hosts/homeserver.json` with padlocks for two keys: "server" and "homeserver's SSH host key". At boot on `homeserver`, sops-nix decrypts it to `/run/secrets/wifi`, a file readable only by its owner.

**Narration:**
> Everything Nix evaluates lands in the store, and every user of the machine can read the store. So a password must never appear in Nix code.
>
> sops-nix solves this. Secrets are stored in the repository encrypted. Each machine can decrypt only its own, with a key derived from its SSH host key. At activation, sops-nix writes each secret to a file under slash run slash secrets, readable only by the user or service that needs it. Your configuration points at that file's path, never at the value.

## Chapter 10: Deploying a fleet with git and comin (about 3:00)

### S10.1 Pull, don't push (1:45)

**Visual:** [MANIM] A git repository on a Forgejo server in the middle. Three machines around it, each with a small comin arrow pulling. A new commit lands on `main`; each comin fetches it, builds `nixosConfigurations.<its own name>`, and switches. A Raspberry Pi icon with a download arrow from a cache.

**On screen:** "A push to main is a deploy"

**Narration:**
> Now put it together for many machines. All their configurations live in one flake, in one git repository, here on a Forgejo server.
>
> Each machine runs comin. Every few minutes comin fetches the repository, and when the main branch has a new commit, it builds the configuration named after its own host name and switches to it. No one connects to the machines to deploy. In other words: a push to main is a deploy, to every machine the commit changes.

### S10.2 Testing branches and moving forward only (1:15)

**Visual:** [MANIM] `main` line with commits. A short branch `testing-pi` sits on top of `main`. Only the Pi follows it, with a "test" door icon (no boot entry). Then `main` advances without it: the Pi drops back to `main`. Last frame: an arrow trying to go back to an older commit, blocked: "comin only moves forward".

**Narration:**
> comin follows a second branch on each machine, testing dash and the machine's name. While that branch sits on top of main, comin deploys it on that one machine only, in test mode. A reboot returns to main. That is the safe way to try a risky change.
>
> comin only moves forward. It refuses to deploy an older commit. So in this world a rollback is not "go back to an old commit", it is a new commit that reverts the change. Hold on to these two ideas, trials and reverts. Part 2 has a button for each.

### S10.3 Recap (0:30)

**Visual:** [MANIM] One frame summarising the whole of Part 1: store → derivations → flake + lock → modules → toplevel → switch → generations; repository → comin → machines; sops for secrets; home-manager for users.

**Narration:**
> That is the whole model. Packages are store paths built from derivations. A flake pins the inputs. Modules merge into one system per machine. Switching activates it, generations let you go back, and comin deploys whatever lands on main. Now let's see the console that manages all of this.

---

# Part 2: nixos-manager

## Chapter 11: What nixos-manager is (about 3:00)

### S11.1 Architecture (2:00)

**Visual:** [MANIM] The deployment diagram from S10.1, plus a new box: **nixos-manager server**. Inside it: "working copy (git)", "Nix (evaluate, check, build)", "web console", "MCP endpoint". Arrows: server ⇄ Forgejo (push, pull, webhook, commit statuses). Machines → server: "deployment reports, heartbeats, closures" (dashed). Server → machines: "binary cache" (dotted, optional). A browser icon and a robot icon (MCP client) talk to the server.

**Narration:**
> nixos-manager is a self-hosted web console for a fleet of NixOS machines that all live in one flake repository.
>
> It keeps a git working copy of that repository, and it runs Nix itself. So it can evaluate every machine and show you, visually, what each one runs and where every setting comes from. When you change something, it shows what the change does to each machine in NixOS terms, then checks it by really building the machines, commits, and pushes to Forgejo. From there, comin deploys as before.
>
> The machines talk back: after each deployment, comin reports what it did, so the console can show you the rollout. And an MCP endpoint gives AI assistants and scripts the same features, under the same rules.

### S11.2 The golden rule (1:00)

**Visual:** [MANIM] A loop of four steps with icons: Edit → Check → Commit → Push (= deploy). A padlock between Check and Commit: "a commit needs a passing check of exactly what is staged". Another between Commit and Push.

**Narration:**
> One rule runs through the whole console: you cannot commit what has not passed a check, and you cannot push what has not passed a check. The check runs on exactly the content you are committing. Only a person logged into the web page can override it, explicitly. Automation cannot. Keep that loop in mind: edit, check, commit, push. Every section that follows is a way to do one of these four steps.

## Chapter 12: Installing the server (about 4:30)

### S12.1 Proxmox LXC, the recommended way (2:30)

**Visual:** [CODE] The `pct create` command from the README, with `--features nesting=1` and `--ostype nixos` highlighted. Then [CODE] `/etc/nixos/configuration.nix`:

```nix
services.nixos-manager = {
  enable = true;
  openFirewall = true; # port 8080
  author = {
    name = "Your Name";
    email = "you@example.com";
  };
  passwordHashFile = "/var/lib/secrets/nixos-manager-password";
};
```

Then [CODE] the password step:

```sh
printf '%s\n' "$pw" | nixos-manager hash-password > /var/lib/secrets/nixos-manager-password
systemctl restart nixos-manager
```

**On screen:** "4 GB RAM to start · 8 GB past a dozen machines · 30 GB disk"

**Narration:**
> The recommended home for nixos-manager is its own NixOS container on Proxmox. Nix is right there, so the console evaluates and builds for real, and the container can later become one of the fleet's machines.
>
> Download the NixOS container template, and create the container with nesting turned on: both systemd's sandboxing and Nix's build sandbox need it. Every machine is a full NixOS evaluation, so memory matters most: four gigabytes is a good start, eight past a dozen machines.
>
> Then import the nixos-manager flake's module and turn the service on. Give it the author every commit will be made as, and the file holding the login password's hash. The package itself creates that hash: pipe the password into nixos manager hash password. Restart the service, and the console is on port eight thousand eighty.

### S12.2 Module options (1:15)

**Visual:** [CODE] A table built from `nix/module.nix`:

| Option | Meaning |
|---|---|
| `enable` | Run the console |
| `package` | The nixos-manager build (the flake sets it) |
| `listen` | Address and port; default `0.0.0.0:8080` |
| `openFirewall` | Open that port |
| `dataDir` | State: working copy, keys, tokens; default `/var/lib/nixos-manager` |
| `author.name`, `author.email` | Identity of every commit |
| `passwordHashFile` | bcrypt hash of the login password |
| `remote` | Repository to clone and push (else set it in Settings) |
| `environmentFile` | Extra secrets as `KEY=value` |
| `extraEnvironment` | Extra variables, e.g. `NIXMGR_EVAL_JOBS = "1"` |
| `checks.build` | `false`: checks only evaluate; each machine builds itself |
| `binaryCache.enable`, `binaryCache.openFirewall` | Serve the store to the machines (port 5000) |

**Narration:**
> These are all the module's options. Most have sensible defaults. Two deserve attention now. Checks dot build: set it to false on a small server, and checks only evaluate and dry-build the machines, while each machine builds its own system when comin deploys. And binary cache: when on, the machines download what the server already built instead of compiling it. Both get their own scene later.

### S12.3 Docker, and the security model (0:45)

**Visual:** [CODE] `compose.yaml` with `NIXMGR_AUTHOR_NAME`, `NIXMGR_AUTHOR_EMAIL`, `NIXMGR_CHECK_BUILD`, the `data-owner` service. A warning card: "Docker: builds can read the server's data. Only when everyone who can push may see the secrets."

**Narration:**
> There is also a Docker image, with its own Nix, running as an unprivileged user. It is simpler, but in a container Nix cannot sandbox builds, so anything the fleet builds could read the server's data, including the key that decrypts the secrets. Use Docker only when everyone who can push to the fleet may see those. Otherwise, use the container setup.
>
> In both cases the console speaks plain HTTP. Keep it on a trusted network, or put it behind Tailscale or a TLS proxy.

## Chapter 13: First steps (about 4:30)

### S13.1 The intro and login (0:45)

**Visual:** [CAPTURE] The animated intro that plays before login (`#/intro`), then the login form. Mention the login rate limit as a lower-third.

**On screen:** "10 attempts / 10 min per address · sessions last 7 days"

**Narration:**
> The first time you open the console, a short animated intro tells this same story. You can watch it again from Settings, About. Then you log in with the password you hashed. Logins are rate limited before the password is even checked, and a browser that has logged in before keeps its own allowance, so someone guessing from elsewhere cannot lock you out.

### S13.2 The starter fleet (1:30)

**Visual:** [CAPTURE] Code view of a fresh repository. [MANIM] The repository tree, each part glowing as it is named:

```
flake.nix
hosts/example/default.nix
hosts/example/hardware-configuration.nix
hosts/example/managed.json
home/pablo.nix
modules/common.nix
modules/managed.nix
modules/report-deployments.nix
modules/secrets.nix
modules/binary-cache.nix
.forgejo/workflows/check.yml
```

**Narration:**
> Without a repository connected, the console starts a new one from a starter fleet. Every folder under hosts with a default dot nix becomes a machine named after the folder. The folder name must equal the machine's host name, because comin builds the configuration with that name.
>
> Each machine folder holds its hand-written default dot nix, its hardware configuration, and managed dot json, where the console writes what you set by clicking. modules slash common dot nix is shared by every machine: it turns on flakes, configures comin, the deployment reports and the binary cache. managed dot nix is the module that applies the managed dot json files. And home slash pablo dot nix is an example home-manager user.

### S13.3 Lock (0:45)

**Visual:** [CAPTURE] Machines view showing "Lock the flake" because there is no `flake.lock` yet. Click **Lock**; the job log streams; `flake.lock` appears in Changes.

**Narration:**
> A new flake has no lock file yet, so nothing can evaluate. Click Lock once. It records the exact revision of every input in flake dot lock. Then stage, check and commit, like any change.

### S13.4 Connecting Forgejo (1:30)

**Visual:** [CAPTURE] Settings → Forgejo, the numbered steps. Paste an SSH URL; show the deploy public key; Forgejo's Deploy keys page with "Enable write access" ticked; **Test connection** with a green result; the webhook URL and secret; the optional commit-status token.

**Narration:**
> Now connect the real repository, in Settings, Forgejo. The steps are numbered.
>
> One: paste the repository's SSH address. Two: the console creates a deploy key. Copy its public half into the repository's deploy keys in Forgejo, with write access. Three: Test connection tells you in plain words what is wrong, if anything: the host, the key, the repository, or a read-only key. If the Forgejo repository already has commits, the console offers to use it instead of the starter.
>
> Four, recommended: a push webhook. Forgejo then tells the console about every push made from anywhere else, so it stays in step, and it checks every pushed commit. Five, optional: a Forgejo token, so each check shows up next to its commit in Forgejo as a commit status.

## Chapter 14: Finding your way around (about 3:00)

### S14.1 The navigation rail (1:15)

**Visual:** [CAPTURE] The left rail, each item highlighted in turn: Fleet, Machines, Packages, Services, Modules, Options, Health, Deployments, Dependencies, Inputs, Changes, History, Code, and Settings at the bottom. Badges: changed-file count on Changes, a busy dot while a check runs, failed deployments on Deployments, findings on Health.

**On screen:** each item with its shortcut: `g f`, `g m`, `g p`, `g s`, `g u`, `g o`, `g a`, `g d`, `g n`, `g i`, `g c`, `g h`, `g e`, `g ,`

**Narration:**
> The rail on the left lists every view. Each has a two-key shortcut: press g, then a letter. g f for Fleet, g m for Machines, g c for Changes, and so on. The rail also carries badges: how many files you have changed, a dot while a check runs, failed deployments, and the number of health findings.

### S14.2 Top bar, branches and the palette (1:45)

**Visual:** [CAPTURE] Top bar: model status, branch menu (switch, create, merge), the ahead/behind counter, "N new on Forgejo", **Pull** and **Push**, the account menu showing the commit author. Then Ctrl+K: the command palette, typing "jelly" finds the Jellyfin service, a machine, an option. Settings → Keyboard shortcuts and Theme (Night, Print).

**Narration:**
> The top bar shows the state of the model, meaning whether the console has evaluated the latest files, and the current branch. The branch menu switches, creates, and merges branches. Next to it, how many commits you are ahead of or behind Forgejo, and Pull and Push. When someone pushes from elsewhere, it tells you how many new commits Forgejo has.
>
> Control K opens the command palette: jump to any machine, package, service, option, file, or action by typing part of its name. Slash searches inside the current view.
>
> Settings lists all shortcuts, and has two themes: Night, the default, and Print, the same console on green-striped paper, for daylight.

## Chapter 15: The fleet graph (about 2:30)

### S15.1 Fleet view (2:30)

**Visual:** [CAPTURE] Fleet view on the fixture fleet. Machine nodes `homeserver`, `pi`, `workstation`, each with chips for enabled services. Connected nodes: services and programs, the files that configure them, package groups (expand "N packages"), users, homes, flake inputs. Click a node: the inspector opens on the right. Deployment marks on machines: deployed, behind, failed, silent.

**Narration:**
> Fleet is the home view: the whole fleet as an interactive graph. Each machine shows the services and programs it enables. Around them are the shared modules and files that configure them, their package groups, their users and homes, and the flake inputs they are built from. When two machines share a service or a file, they connect to the same node, so you see at a glance what they have in common.
>
> Click any node to open the inspector: which machines use it and from which file. A machine node also shows its deployment state: deployed, behind, failed, or silent. This is the map. The rest of the console is where you act on it.

## Chapter 16: Machines and new machines (about 6:00)

### S16.1 The Machines view (1:00)

**Visual:** [CAPTURE] Machines view: one card per machine; the Flake inputs block with **Update** and "Update all inputs"; "Home configurations" section with `pablo@laptop`; buttons **New machine** and **Installer ISO**.

**Narration:**
> Machines lists every NixOS configuration in the flake as a card, and below them any standalone home-manager configurations. From here you open a machine's page, create a new machine, or build an installer for one.

### S16.2 The New machine wizard (2:30)

**Visual:** [CAPTURE] **New machine**: name `nas`; role: Minimal, Server, Desktop; platform: physical machine, Proxmox VM, Proxmox LXC; the **User** step (name, SSH keys, administrator, home-manager); hardware configuration paste box; boot loader choice (UEFI systemd-boot, or BIOS GRUB on a disk); disk layout (ext4 or Btrfs). Then Changes showing the new `hosts/nas/` files.

[MANIM] side card with the role presets, from `internal/core/machines.go`:

| Role | Packages | Settings |
|---|---|---|
| Minimal | git, htop | none |
| Server | btop, git, htop, tmux | SSH on with keys only and no root login, fail2ban, firewall, weekly GC keeping 14 days, store optimisation |
| Desktop | firefox, git, htop, vlc | Plasma 6 with SDDM, PipeWire, printing, NetworkManager, Bluetooth, zram, weekly GC, store optimisation |

**Narration:**
> New machine writes a new folder under hosts. Give it a name, which becomes its host name, and pick a role. Minimal adds only git and htop. Server turns on SSH with keys only and no root login, fail to ban, the firewall, and weekly garbage collection. Desktop adds Plasma, sound, printing, network manager and Bluetooth. Everything a role sets lands in managed dot json, so it stays editable by clicking.
>
> Pick the platform: a physical machine, a Proxmox virtual machine, or a Proxmox container, which needs no hardware configuration.
>
> No role creates a user, and the server role only accepts SSH keys, so the user step asks for the first one: a name, its public keys, whether it is an administrator, and whether home-manager is on for it.
>
> For a physical machine or a VM, either paste the hardware configuration from the NixOS installer and pick its boot loader, or choose a disk layout, ext four or Btrfs, and let the machine be installed for you. Nothing is committed until you review it in Changes.

### S16.3 Installing it: nixos-anywhere (1:00)

**Visual:** [CAPTURE] The machine page's **Install with nixos-anywhere** card: address box, **Fetch host key**, the fingerprint, **Use it for secrets**, then the generated command:

```sh
nix run github:nix-community/nixos-anywhere -- --flake .#nas \
  --target-host root@192.168.1.50 --copy-host-keys \
  --generate-hardware-config nixos-generate-config ./hosts/nas/hardware-configuration.nix
```

**Narration:**
> A machine with a disk layout gets an install card. Boot it from any NixOS installer with root SSH, enter its address and fetch its host key. The console only reads the key the machine proves it holds: it sends no password and runs nothing. Compare the fingerprint, then use it for secrets, which lets the machine decrypt its own from the first boot.
>
> The card then gives you the nixos anywhere command to run from a computer with Nix. It erases the disk, installs the machine, keeps that host key, and commits the real hardware configuration back. The console itself never runs anything on the machine.

### S16.4 Installing it: the installer ISO (1:30)

**Visual:** [CAPTURE] Machines → **Installer ISO**: pick `nas`, **Build ISO**, live log, then **Download** with the SHA-256 and the install steps. [MANIM] the boot sequence: USB → `sudo fleet-install` shows addresses and host key → console fetches and approves the key → Enter → type the machine name → disks partitioned → `nixos-install --flake` → passwords → hardware config pushed → reboot → comin.

**On screen:** "Nothing secret is on the ISO"

**Narration:**
> Or build an installer ISO for that one machine. The console builds a minimal NixOS installer from the machine's own pinned nixpkgs, with the pushed commit on it. Write it to a USB stick and boot the machine.
>
> Run sudo fleet install. It shows the machine's addresses and the SSH host key it will keep. In the console, fetch that key, compare it, and approve it. Press Enter: it fetches the repository, shows the disks, and once you type the machine's name, partitions them, installs, offers to set the users' passwords, pushes the hardware configuration, and reboots.
>
> The machine comes up already enrolled: comin pulls, its secrets open, and it shows up in Deployments. Nothing secret is ever on the stick.

## Chapter 17: The machine page (about 13:00)

### S17.1 Overview (1:00)

**Visual:** [CAPTURE] `#/machines/homeserver/overview`. The tab strip: Overview, Essentials, Packages, Dependencies, Services & programs, Modules, Users, Home, Network & storage. Overview cards: "Configured by" (files), services, users and access, file systems, secrets the machine decrypts, the Deployment card.

**Narration:**
> Open a machine and you get its page, with nine tabs. Overview summarises it: the files that configure it, the services it runs, its users and how they log in, its file systems, the secrets it decrypts, and its deployment: which commit it runs and when it was deployed.

### S17.2 Essentials (1:30)

**Visual:** [CAPTURE] Essentials tab, groups lit in turn: time zone; language and formats; console keyboard; automatic garbage collection with schedule and "keep generations for"; store optimisation; zram; firewall; SSH server, password logins, root login; fail2ban; Tailscale. Each row shows its real value and a "set in" file tag. Toggle **fail2ban** on.

**Narration:**
> Essentials gathers the settings almost every machine needs, each with its real value and the file that sets it: time zone, language, keyboard, garbage collection and how many days of generations to keep, store optimisation, compressed swap in memory, the firewall, the SSH server with password and root logins, fail to ban, and Tailscale.
>
> Flip a switch and the value goes into this machine's managed dot json. Nothing reaches the machine yet: it is a change in the working copy, which you will check, commit and push.

### S17.3 Packages: add, remove, replace (2:00)

**Visual:** [CAPTURE] Packages tab. Each package row with its source tag: Default (NixOS), a file, "✎ Set here", "⇉ Every machine". **Add packages**: search "ripgrep" from the locked nixpkgs, add. **Remove** on `nano` (a default): it shows "⌫ Removed here" with **Restore**. **Replace** `nodejs_20` with `nodejs_24`.

**Narration:**
> Packages lists everything installed on the machine's system, with where each one comes from: a NixOS default, a hand-written file, a flake input, or a click here.
>
> Add packages searches the exact nixpkgs your flake locks, so what you find is what the machine will get. Remove works on any package, even one a hand-written file or NixOS itself adds: the console records the name as removed in managed dot json and leaves the Nix that adds it untouched. Restore gives it back. A package the system needs, like bash, asks before it goes.
>
> Replace removes a package and installs another in its place, for example node twenty replaced with node twenty four.

### S17.4 Packages: customise (2:00)

**Visual:** [CAPTURE] **Customise** drawer on `btop`: sections "From another nixpkgs" (pick `nixpkgs-unstable`), override arguments, overrideAttrs (`doCheck = false`), version and source (GitHub tag, empty hash), patches, and "Nix file". Then **Reset to default**. [MANIM] overlay diagram: the machine's package set with an overlay layer replacing `btop`, and everything depending on it rebuilt.

**Narration:**
> Customise changes how a package is built on this machine. You can take it from another nixpkgs, such as unstable, for a newer version: add that input once, with the line the drawer shows you, and Lock. You can pass different arguments, change build attributes, like skipping its tests, pin a version and source, from a URL or a GitHub tag, or add patches. Leave the hash empty and the check fails with the right one in its log.
>
> For anything else, it opens a Nix file of its own for that package. All of this becomes an overlay, so everything on the machine that uses the package gets the customised one. Reset to default undoes it.

### S17.5 Every machine, Copy to, and Nix for a machine (1:15)

**Visual:** [CAPTURE] The tab's switch between this machine and **Every machine**; add `ncdu` for every machine; the row shows "⇉ Every machine". **Copy to…** in the Customise drawer, ticking `pi`. **Nix for homeserver** opening `hosts/homeserver/custom.nix` in Code.

**Narration:**
> Switch the tab to Every machine, and the same actions write modules slash managed dot json instead, which applies to every machine. Where a machine sets something itself, the machine's own value wins.
>
> Copy to writes one machine's change to other machines you pick. It never overwrites: a machine that already has something different is left alone, and the result tells you why.
>
> And for anything the console does not model, Nix for this machine opens its own custom dot nix file, a plain NixOS module, in the code editor.

### S17.6 Dependencies tab (0:45)

**Visual:** [CAPTURE] Dependencies tab: the closure as a tree from the system path, search "openssl", select it: version, size, "Used by", and **Why it is here**, a chain from the system down to it.

**Narration:**
> Dependencies shows the closure the machine actually runs, as reported by the machine itself, like the nix tree tool. Search for a library, see its version and size, what uses it, and, best of all, why it is here: the chain of references from the system down to it.

### S17.7 Services and programs (1:00)

**Visual:** [CAPTURE] Services & programs: enabled features with the files that set them; **Add service or program** opens the catalog "Popular services" grouped by category: Media (Jellyfin, Immich), Files (Syncthing, Samba), Home (Home Assistant), Network (AdGuard Home, mDNS), Containers (Docker, Podman, Virtual machines), Monitoring (Cockpit, node exporter, smartd), Data (PostgreSQL), AI (Ollama, Open WebUI), Desktop (Flatpak, Bluetooth, Firmware updates). Add Jellyfin; then **Configure** opens its option form.

**Narration:**
> Services and programs lists everything the machine enables, with the file that turns it on. Add service or program opens a catalog of popular services, from Jellyfin and Immich to Home Assistant, Docker, Ollama and more. Adding Jellyfin, for example, sets services dot jellyfin dot enable and opens its firewall port. Configure then opens a form for every option the service has.

### S17.8 Modules tab (0:30)

**Visual:** [CAPTURE] The machine's Modules tab: a switch per fleet module and a field per module option, with **Reset**.

**Narration:**
> The Modules tab turns the fleet's modules on or off for this machine, and sets their options. We'll see what a fleet module is in a moment.

### S17.9 Users (1:00)

**Visual:** [CAPTURE] Users tab: users with groups and "Authorized SSH keys (one per line)"; **Add** user, **Add key**, **Remove user**. Note banner: "groups and keys are written as whole lists".

**Narration:**
> Users lists the machine's users, their groups and their authorized SSH keys. You can add a user, add a key, or remove a user created here. Groups and keys are written as whole lists, which win over the hand-written ones, so the form always starts from the complete current list. Before removing password logins anywhere, make sure a key is here.

### S17.10 Home (2:00)

**Visual:** [CAPTURE] Home tab for `pablo`: **Turn on home-manager**; Packages (with Every machine switch); Options (search "git", set `programs.git.enable`); Files in ~ (create `.config/kitty/kitty.conf`); Secrets in ~ (link `~/.netrc` to secret `netrc`; a template `.config/gh/hosts.yml` with `oauth_token: @secret:gh-token@`); Apps; **Nix for pablo**.

**Narration:**
> Home manages each user's home-manager configuration on this machine. Turn it on, then add packages, set any home-manager option from a search, and keep dotfiles under Files in your home folder, which become read-only links.
>
> Those files go into the Nix store, so never put a token in them. Use Secrets in your home folder instead: link a whole secret file, like your dot netrc, or write a template where at secret, name, at stands for a secret. sops-nix fills it in on the machine, readable only by you.
>
> And as for the system, Nix for this user opens a home-manager module of the user's own.

### S17.11 Home: Apps (1:15)

**Visual:** [MANIM] The four-kind table from the README:

| Kind | Example | Managed as |
|---|---|---|
| A file the app never writes | kitty, Waybar | Files in ~, linked read-only |
| A file the app also writes | Supersonic `config.toml` | App settings file: only the keys set here |
| A password in the keyring | Supersonic's server password | Keyring entry |
| Opaque or rotating storage | Discord login, OAuth tokens | Kept in the app only |

Then [CAPTURE] Apps section: an app settings file with format TOML, mode merge, "wait while running" = `supersonic`.

**Narration:**
> Some desktop apps rewrite their own settings file, and a read-only link would break them. The Apps section handles each case with the right tool. For a file the app also writes, it sets only the keys you choose and keeps the app's own, in JSON, TOML, YAML, INI or properties files. It can wait while the app runs, so the app does not overwrite the change on exit. A password can go into the desktop keyring. And what apps keep to themselves, like login sessions and rotating tokens, is listed but never touched.

### S17.12 Network & storage (0:45)

**Visual:** [CAPTURE] Network & storage: Firewall, every open port as evaluated with who opened it, the "Allowed TCP ports" list with **Open port** and **Reset**, file systems.

**Narration:**
> Network and storage shows every open port, whoever opened it, a service with open firewall, hand-written Nix, or this list. Opening a port here sets the whole list, keeping the ports already in it. Below it, the machine's file systems.

## Chapter 18: Packages and Services matrices (about 1:00)

### S18.1 Matrices (1:00)

**Visual:** [CAPTURE] Packages view: rows = packages, columns = machines, with "⇉ Every machine" and "✎ Set here" marks. Services view: same for services. Highlight a gap: `htop` on every machine except `pi`.

**Narration:**
> The Packages and Services views are matrices: which package, and which service, runs on which machine. They make gaps obvious, like a tool you installed everywhere except on one machine, which Health also points out.

## Chapter 19: Fleet modules (about 3:00)

### S19.1 Why and how (3:00)

**Visual:** [CAPTURE] Modules → **New module** `media`. Its page: **Machines** (switch per machine and one for every machine), **Options** (`port`, type port, default 8096), **Packages** (`ffmpeg`), **Settings** (`services.jellyfin.enable = true`, `networking.firewall.allowedTCPPorts = ["@option:port@"]`), **Add Nix**, **Delete**. [CODE] `modules/fleet/media/managed.json`. [MANIM] It becomes options `fleet.modules.media.enable` and `fleet.modules.media.port`, with settings applied under `mkIf enable`.

**Narration:**
> A fleet module bundles packages and settings, a media server, a desktop, a build machine, as a NixOS module of its own, and turns it on machine by machine with one switch.
>
> Create a module called media. Give it an option, port, of type port with a default of eight oh nine six. Add the ffmpeg package. In its settings, enable Jellyfin and open the firewall port, using at option port at, which means "this machine's value of the port option".
>
> Behind the scenes it becomes a real NixOS module with the option fleet dot modules dot media dot enable, and one option for port. A module's settings apply like hand-written Nix, so its firewall port joins the machine's own. On each machine, turning it on writes that enable option into its managed dot json, checked and pushed like anything else. An option without a default must be set by every machine that uses the module, and the check names the one that forgot.
>
> Add Nix opens a custom dot nix next to it, for what JSON cannot express. Delete removes the module and every setting that referred to it.

## Chapter 20: Options (about 2:30)

### S20.1 Option browser and forms (1:30)

**Visual:** [CAPTURE] Options view: pick `homeserver`; search "openssh"; select `services.openssh.settings.PermitRootLogin`: its type, default, description, the value on this machine and where it is set. **Edit as a form** / **Edit as JSON**; **Use a secret…** on a file-path option; a read-only option ("NixOS computes this option"); "Code only" for an option inside a list of submodules.

**Narration:**
> Options browses every NixOS option the machine can set, from its own evaluation, so options from your flake inputs are there too. Select one to see its type, its documentation, its real value on this machine, and the file that sets it.
>
> Set it with a form built from the option's type, or as JSON. Where an option takes a file path, like a password file, the form offers the machine's secrets instead. Some options can't be set here: ones NixOS computes, and ones inside a list of submodules, which you set in Code.

### S20.2 Compare and copy (1:00)

**Visual:** [CAPTURE] The option form's **On every machine** section for `time.timeZone`: `homeserver`, `pi`, `workstation`, each with its value and the file that sets it; **Copy to…** on one row, tick `pi`, "Copy to 1 machine". A "lib.mkForce" note on a value that a click cannot beat.

**Narration:**
> Under the form, On every machine shows the option's value on each machine, side by side, with the file that sets it. Copy to sends one machine's value to the machines you pick. If a value is set with make force in hand-written Nix, the console tells you, since a click can't override it.

## Chapter 21: Health (about 3:00)

### S21.1 The findings (3:00)

**Visual:** [CAPTURE] Health view with findings, each with severity and **Fix** where available. [MANIM] the list scrolls as it is narrated:

- Machine does not evaluate (error)
- Firewall disabled → Turn the firewall on
- SSH accepts passwords → Allow SSH keys only
- SSH allows root logins → Forbid root logins
- SSH without fail2ban → Enable fail2ban
- No automatic garbage collection → Collect weekly, keep 14 days
- Store not optimised → Optimise the store automatically
- Input N days old → Update input
- Release at or near end of life; mixed releases; input following an ended release
- Visual editing off; managed.nix older than this console → Update managed.nix
- Package on every machine except one
- No comin, or comin pulling another repository
- Last pushed commit not checked → Check
- Deployment failed, behind for over an hour, silent, changed by hand, switch unfinished, reboot pending
- Machines still using the shared report token, no heartbeat, never reported

**Narration:**
> Health is the fleet's checklist. It flags machines that don't evaluate, a disabled firewall, SSH that accepts passwords or root logins, SSH without fail to ban, no automatic garbage collection or store optimisation, old flake inputs, NixOS releases at or near their end of life, and a package that is on every machine except one.
>
> It also watches delivery: a machine without comin, or comin pulling a different repository than the one the console pushes to, which means your pushes never reach it. A last commit on main that has not passed a check. And deployment problems: failed, behind for over an hour, silent, changed by hand, or waiting for a reboot.
>
> Most findings have a Fix button. Allow SSH keys only, for example, sets password authentication to false on every affected machine. As always, it is a change you then check, commit and push. Before you press that one, make sure each machine has an SSH key.

## Chapter 22: Changes, checks, commits (about 7:00)

### S22.1 The diff (1:00)

**Visual:** [CAPTURE] Changes view after enabling fail2ban on `homeserver` and adding Jellyfin on `pi`: changed files with their text diff, stage and unstage per file, discard.

**Narration:**
> Every click so far went into the working copy. Changes is where you review it. On the left, the files that differ from the last commit, with a text diff. Stage the ones you want in this commit, or discard what you don't.

### S22.2 On the machines: the model diff (2:00)

**Visual:** [CAPTURE] "On the machines" panel: `pi` changed: package `jellyfin` added, service `jellyfin` started, port 8096 opened, `/etc` files added; `homeserver` changed: service `fail2ban` added; `workstation` unchanged. A "Reboot needed" badge example; units grouped as restart, reload, start, stop, "Not restarted"; "Other configuration" for a sysctl. After a check: closure size before and after and version changes.

**On screen:** "A machine changes when its system toplevel changes"

**Narration:**
> Below the diff is the part that makes this console different: what the change does on each machine, in NixOS terms. A machine counts as changed when its system toplevel changes, the idea from chapter six.
>
> For each changed machine you see the packages and versions added or removed, services, users, ports, and settings. Which systemd units a switch will restart, reload, start or stop, decided the way switch to configuration decides. The files in slash etc that change. Whether it needs a reboot, for a new kernel or kernel parameter. And after a check has built it, every dependency whose version or size changes, and the closure size before and after. Machines that don't change are listed as unchanged, so you know for sure the change stays where you meant it.

### S22.3 Checks (2:00)

**Visual:** [CAPTURE] Checks panel: **Run check**; live log: `nix flake check`, then dry run (what will be built and downloaded), then a build of `pi` and `homeserver` only; result: built, failed, reboot needed. Tick other configurations (**All**, **None**) for a dry build. [MANIM] a commit line: "newest pushed commit that passed a check" vs staged content; only machines whose toplevel differs are built.

**Narration:**
> Run the check on what is staged. It runs nix flake check, then really builds every machine whose system changes compared with the newest pushed commit that passed a check. So a package that evaluates but fails to build fails the check, here, before any machine sees it. The log streams live, and the result says which machines were built, which failed, and which need a reboot. You can tick other configurations to dry-build them too.
>
> On a small server, set checks dot build to false: the check then evaluates and dry-builds, and each machine builds its own system when comin deploys it. Checks run one at a time in their own queue, so a long one never blocks the rest of the console.

### S22.4 Commit (0:45)

**Visual:** [CAPTURE] Commit message `homeserver: enable fail2ban`, Ctrl+Enter; History shows the commit with trailer `Via: web session 3fa2c1d9`.

**Narration:**
> Once the check passes, write a message and commit. Every commit is made as the configured author, with a trailer saying how it arrived: through a web session, or through a named API token. So history always tells you who, or what, made each change.

### S22.5 Push (1:15)

**Visual:** [CAPTURE] **Push** dialog: what each machine gets compared with what is already pushed, the commits included; confirm. [MANIM] the commit travels to Forgejo; comin arrows light up on `homeserver` and `pi`; `workstation` stays dim.

**Narration:**
> Push opens one last summary: what each machine gets, compared with what is already pushed. Confirm, and the commit goes to Forgejo. From here comin takes over, and only the machines that changed will switch.
>
> If Pull ever rebases your local commits on top of new ones from Forgejo, that combination was never checked as a whole, so Push refuses it. Run the check again with nothing staged, then push.

## Chapter 23: Trying a change on one machine (about 2:30)

### S23.1 Try on, promote, end the trial (2:30)

**Visual:** [CAPTURE] A risky change: a firewall change on `pi`. Stage, check, then **Try on pi** next to Commit. The console moves to branch `testing-pi`; Changes shows how the machine takes it. Then **Promote to main** (stages the testing branch on main as one change) and, alternatively, **End the trial**. [MANIM] reuse S10.2.

**Narration:**
> For anything risky, firewall, network, VPN or SSH, try it on one machine first. Stage the change, run the check, then click Try on pi. The console commits it on the testing pi branch, on top of main, and pushes that branch. Only the Pi deploys it, in test mode, so a reboot brings back what main gave it.
>
> Watch it in Changes. If it works, Promote to main stages the trial on main as one change: commit and push it, and every machine gets it. If not, End the trial points the testing branch back at main, and comin switches the Pi back.
>
> Why does this matter? comin pulls from the machine. A machine that loses its network cannot pull the fix. With a trial, a reboot is always enough.

## Chapter 24: Deployments and rollback (about 5:00)

### S24.1 Turning reporting on (1:15)

**Visual:** [CAPTURE] Deployments view setup steps. [CODE] `fleet.deployReport.url = "http://nixos-manager.home.lan:8080";` in `modules/common.nix`. The per-machine token and the command:

```sh
echo 'nxh_…' | ssh root@pi 'install -D -m 600 /dev/stdin /var/lib/nixos-manager-report/token'
```

Or Settings → Secrets → **Store every machine's token**.

**Narration:**
> To watch deployments, the machines need to report back. In modules slash common dot nix, set the address the machines reach the console at. Then give each machine its own report token. The Deployments view shows the token and the command to put it on the machine. Or, with secrets set up, store every machine's token as a secret in one click. A token only reports for its own machine, so one machine cannot pass for another.

### S24.2 Watching a rollout (1:30)

**Visual:** [CAPTURE] Deployments view: latest commits of `main`, each with the machines that run it; per machine: commit, deployed time, comin's error if any. States: current, behind, failed, silent, never. Verdicts: "Changed by hand", "Switch unfinished", "Reboot pending". **Forget** on a row.

**Narration:**
> Deployments shows the latest commits of the branch with the machines running each one. After a push, you watch them catch up. Per machine: the commit, when it was deployed, and comin's error if a build or switch failed.
>
> Every fifteen minutes each machine also sends a heartbeat. One that stays quiet longer than two hours is flagged as silent, even if it was up to date. For a laptop that is often off, set silent after to null. The console also compares what each machine runs with what its commit should build, and tells you when it was changed by hand, when a switch didn't finish, or when a reboot is pending.

### S24.3 Rolling back (1:30)

**Visual:** [CAPTURE] A failed deployment on `pi`: **Revert the commit that broke pi**, the dialog listing the culprit commits; it stages the revert and starts the check; **Commit** → push dialog. [MANIM] the commit line moving forward with a revert commit, never backwards.

**Narration:**
> When a deployment fails on main, the machine's row offers Revert the commit that broke it. It finds the commits after the last one the machine deployed, up to the failing one, stages a revert, and starts the check. Commit and push, and every machine gets the fix.
>
> There is no "pin a machine to an old commit". comin only moves forward, so a rollback is a revert: a new commit with the old configuration, kept in history. If a change cut a machine off the network, push the fix first, then bring the machine back from its console: pick the previous generation in the boot menu, and comin picks up the fix once it is online.

### S24.4 Notifications (0:45)

**Visual:** [CAPTURE] Settings → Notifications: ntfy, webhook, email (SMTP); **Send a test message**. A phone mock-up receiving "pi failed to deploy a3f19c2".

**Narration:**
> You don't have to watch. Settings, Notifications sends a message through ntfy, a webhook, or email when a machine fails to deploy, stays behind for an hour, goes silent, when main no longer evaluates, when a release nears its end of life, or when a scheduled update is ready.

## Chapter 25: Dependencies across the fleet (about 1:00)

### S25.1 Dependencies view (1:00)

**Visual:** [CAPTURE] Dependencies view: every package and library against the machines with their versions; **Versions differ** filter; click a cell to open that machine's tree at the entry.

**Narration:**
> The Dependencies view puts every machine's closure side by side: every package and library, with the version each machine has. Filter to versions that differ, for example to find which machine still runs an older openssl, and click a cell to jump into that machine's tree.

## Chapter 26: Flake inputs and scheduled updates (about 3:00)

### S26.1 Inputs (1:15)

**Visual:** [CAPTURE] Inputs view: each input with revision and age (amber over 30 days, red over 60); **Lock the flake**; **Update** one input; "What the update changes": inputs that moved with compare links, and package versions per machine.

**Narration:**
> Inputs lists every flake input with its revision and age. Amber after thirty days, red after sixty. Update one input, or all of them, and the console shows what moved, with links to compare, and which package versions change on which machine. Then review and commit it like any change.

### S26.2 Scheduled update (1:45)

**Visual:** [CAPTURE] Inputs → **Scheduled update**: weekly on Sunday at 04:00, server time zone shown; **Update now**; result card: moved inputs, per-machine versions, kernel, reboot needed; **Merge into main**; **Discard**. [MANIM] a `flake-update` branch next to `main` that no machine follows; merge is a fast-forward.

**Narration:**
> Better still, let the console do it on a schedule: every week, or every day, at an hour you choose. It updates every input on top of what the machines deploy, commits the new lock on a branch called flake update, which no machine follows, and checks it. A notification says it is ready.
>
> Inputs then shows what moved, the versions per machine, the kernel, and whether a reboot is needed. Nothing reaches the machines until you click Merge into main. Or discard it.

## Chapter 27: History and Code (about 2:30)

### S27.1 History (1:15)

**Visual:** [CAPTURE] History: commits with their per-machine model diff; **Show**; revert a commit; file history of `hosts/pi/managed.json` with diff and **restore**.

**Narration:**
> History lists every commit, and for each one, what it did to each machine, in the same terms as Changes. Revert any commit, which stages the opposite change for you to check. Or open a single file's history and restore an older version of it.

### S27.2 Code (1:15)

**Visual:** [CAPTURE] Code view: file tree, `hosts/homeserver/default.nix` open; completion from nixd while typing `services.jelly`; hover documentation; a red diagnostic; Shift+Alt+F formats with nixfmt; Ctrl+S saves.

**Narration:**
> Code is a full editor for the hand-written files. It has completion, diagnostics and option documentation from the nix D language server, which evaluates your actual flake, and formatting with nixfmt. Control S saves, shift alt F formats. The console never rewrites your hand-written Nix: clicks go to managed dot json, and your files stay yours.

## Chapter 28: Secrets (about 4:00)

### S28.1 Layout and keys (2:00)

**Visual:** [MANIM] The secrets layout:

```
.sops.yaml                  who can open each file
secrets/common.json         every machine with a key
secrets/hosts/<name>.pub    a machine's SSH host key (public)
secrets/hosts/<name>.json   that machine alone
```

[CAPTURE] Settings → Secrets: the server key notice ("Back it up"), machine keys, **Keys waiting for approval**, **Re-encrypt**, **Remove**.

**On screen:** "Back up `$NIXMGR_DATA/sops-age-key`"

**Narration:**
> Settings, Secrets keeps the fleet's secrets with sops. Values shared by every machine go in secrets slash common dot json; each machine's own in a file named after it.
>
> The server has its own age key, created the first time it's needed. Back it up: without it, nobody can change the secrets. Each machine gets secrets once its SSH host key is approved here. Approval is a person's decision: a key that arrives in the repository any other way, through a pull, or an edit token, waits under Keys waiting for approval, and nothing is encrypted for it until someone approves it.

### S28.2 Values and use (2:00)

**Visual:** [CAPTURE] Add secret `wifi` for `pi`; the value field; it is stored encrypted; **Show** reveals it only to the logged-in person. Then [CODE]:

```nix
sops.secrets.wifi.owner = "wpa_supplicant";
networking.wireless.secretsFile = config.sops.secrets.wifi.path;
```

And Settings → Secrets → **Private repository** (comin token) and **Store every machine's token**.

**Narration:**
> To add a secret, give it a name and a value. The value goes straight to sops and only the encrypted form is written. Show decrypts one value for the person who asked, and nothing keeps it. No MCP tool can ever read a value.
>
> On the machine, each secret becomes a file under slash run slash secrets. Point an option at its path, as here for the Wi-Fi password, or pick it from the option form, which offers secrets wherever a file path goes.
>
> Two setup steps live here too. Private repository stores a read-only Forgejo token, so comin can pull a private repository. And Store every machine's token saves each machine's report token as its own secret. Every change here is a change in the working copy: check, commit and push.

## Chapter 29: Binary cache (about 1:30)

### S29.1 Serving what checks built (1:30)

**Visual:** [MANIM] A check builds `pi`'s system on the server; with the cache on, `pi` downloads it over port 5000 instead of compiling. [CODE] server side `services.nixos-manager.binaryCache = { enable = true; openFirewall = true; };` and fleet side `fleet.binaryCache = { url = "http://nixos-manager.home.lan:5000"; publicKey = "nixos-manager-1:…"; };`.

**Narration:**
> A check already builds the systems a change touches. Turn on the binary cache, and the server serves its store to the machines, signed with its own key, so a Raspberry Pi downloads its new system instead of compiling it. Enable it in the server's configuration, then set its address and public key in the fleet's common module. A machine away from home simply falls back to building after a five second timeout.
>
> For an ARM machine like the Pi on an x86 server, the server needs emulation or a remote ARM builder to build its system; the README has both setups.

## Chapter 30: Standalone homes and upgrades (about 1:30)

### S30.1 Standalone homes, managed.nix versions (1:30)

**Visual:** [CAPTURE] Machines → Home configurations → `pablo@laptop`: Packages, Options, Files in ~, Nix; **Turn on editing** card with the `homeConfigurations` line. Then Health: "modules/managed.nix is older than this console" → **Update managed.nix** and its diff in Changes.

**Narration:**
> A standalone home, for a computer that isn't NixOS or isn't managed here, gets a page of its own, with packages, options, files and Nix, kept in homes slash its name. Deploy it on that computer with home manager switch.
>
> As the console gains features, the managed dot nix module in your fleet gets new versions. Health tells you when yours is older, and Update managed dot nix writes the new one for you to review in Changes.

## Chapter 31: Automation with MCP and API tokens (about 3:00)

### S31.1 Tokens and scopes (1:15)

**Visual:** [CAPTURE] Settings → API tokens: create a token named `ci`, scope **Read only** / **Edit, no push** / **Full**, optional expiry; revoke. [MANIM] three nested rings: read ⊂ edit ⊂ full, with examples on each ring.

**Narration:**
> Settings, API tokens creates named tokens for scripts and AI assistants. Each has a scope. Read only sees files, history, the fleet and deployments. Edit, no push can also edit, stage, check, commit and write secrets, but cannot deploy. Full can also push, merge an update, read a secret's value, and approve machine keys. Give each client the smallest scope it needs, and an expiry if it's temporary.

### S31.2 The MCP endpoint (1:45)

**Visual:** [CODE] the client configuration:

```json
{
  "mcpServers": {
    "nixos-manager": {
      "type": "http",
      "url": "http://<server>:8080/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

[MANIM] the 63 tools as tiles in groups: Fleet (fleet_overview, fleet_insights, host_details, model_diff, compare_option, search_packages, search_options, …), Packages and options (add_packages, remove_packages, edit_packages, set_package_override, set_option, unset_option, copy_to_machines), Homes (set_home_manager, link_home_secret, set_home_config…, set_keyring_entry…), Modules (list_modules, create_module, edit_module, delete_module, update_managed_module), Machines (create_machine), Files (list_files, read_file, write_file, format_nix, …), Git (status, diff, stage, commit, log, revert_commit, merge_branch, push, pull, …), Checks and inputs (list_targets, run_check, flake_lock, flake_update), Deployments (deployments, try_on_machine, end_trial, machine_dependencies, fleet_dependencies), Secrets (list_secrets, set_secret, delete_secret).

**Narration:**
> Point any MCP client at slash m c p with a token, and it gets the console's features as tools: sixty three of them, from fleet overview and model diff to run check, push, and try on machine. The endpoint only offers the tools the token's scope allows.
>
> They call the same code as the web page, under the same rules. A client must read a file before writing it, so it never overwrites an edit it hasn't seen. And it can never commit or push without a passing check. That is how an AI assistant can safely operate the fleet: it proposes, checks and deploys through the same gate you do, and every commit it makes says so in its trailer.

## Chapter 32: Putting it together (about 3:30)

### S32.1 Five everyday recipes (3:00)

**Visual:** [CAPTURE] Fast montage, one recipe each, with a lower-third naming the steps.

1. Install a tool on one machine: Machine → Packages → Add packages → Changes → Check → Commit → Push → Deployments.
2. Run a service: Machine → Services & programs → Add service → Configure → Check → Commit → Push.
3. Risky network change: Stage → Check → Try on <machine> → watch → Promote to main.
4. Something broke: Deployments → Revert the commit that broke <machine> → Check → Commit → Push.
5. Stay up to date: Inputs → Scheduled update → notification → review → Merge into main.

**Narration:**
> Let's put it together with five everyday tasks.
>
> To install a tool on one machine: add the package on its Packages tab, review it in Changes, check, commit, push, and watch it in Deployments.
>
> To run a service: add it from the catalog, adjust its options, then the same loop.
>
> For a risky change to the network or SSH: check it, try it on that one machine, watch it, then promote it to main.
>
> When something breaks: revert the commit that broke the machine, straight from Deployments, check, commit, push.
>
> And to stay up to date: let the scheduled update prepare, check, and summarise new inputs, then merge it when you're happy.

### S32.2 Closing (0:30)

**Visual:** [MANIM] The fleet diagram from the cold open, now with the console in the middle; the snowflake mark of the nav rail.

**Narration:**
> Every machine described in one repository. Every change shown in NixOS terms, checked by a real build, and deployed by a push. And every step reversible. That's Nix, NixOS, and nixos-manager.

---

## Production notes

### Capturing the console without a real fleet

The repository ships a stand-in for Nix and a fixture fleet, made for UI work:

```sh
cd web && npm ci && npm run build && cd ..
go build ./cmd/nixos-manager
NIXMGR_NIX=$PWD/scripts/dev/fake-nix NIXMGR_FAKE_DELAY=1 \
NIXMGR_AUTHOR_NAME=Demo NIXMGR_AUTHOR_EMAIL=demo@example.com \
NIXMGR_PASSWORD_HASH="$(printf 'demo\n' | ./nixos-manager hash-password)" \
NIXMGR_DATA=$PWD/.demo ./nixos-manager
```

- It answers evaluations with four configurations from `internal/core/testdata/fleet/`: machines `homeserver`, `pi`, `workstation`, and the standalone home `pablo@laptop`. `NIXMGR_FAKE_DELAY=1` shows the progress states.
- Check the exact flags against `scripts/dev/fake-nix` and the README's Development section before recording **[volatile]**.
- Real checks, builds, ISOs and closures are stubbed or absent with fake Nix. For S22.3 (check log), S16.4 (ISO), S17.6 and S25.1 (dependency trees), S24.2 (deployments) and S26.2 (scheduled update), either record against a real test instance, or mock the panels with plausible data. Never record Pablo's real console: it shows real host names, the remote URL, tokens on the Tokens page, and the webhook secret.
- Never show a secret value, a token, a webhook secret or a deploy private key on screen, even fake-looking ones, unless they are obviously placeholders.

### Capture list by route

| Scene | Route |
|---|---|
| S13.1 | `#/intro`, login |
| S14 | any view; Settings `#/settings` |
| S15 | `#/fleet` |
| S16.1-S16.4 | `#/machines`, New machine, Installer ISO |
| S17.x | `#/machines/homeserver/<tab>` with tab `overview`, `essentials`, `packages`, `dependencies`, `services`, `modules`, `users`, `home`, `network` |
| S18 | `#/packages`, `#/services` |
| S19 | `#/modules` |
| S20 | `#/options` |
| S21 | `#/health` |
| S22-S23 | `#/changes` |
| S24 | `#/deployments` |
| S25 | `#/dependencies` |
| S26 | `#/inputs` |
| S27 | `#/history`, `#/code` |
| S28-S31 | `#/settings` |

### Manim scenes to build

S0.1, S0.2, S1.1, S1.2, S2.1, S2.2, S3.1, S3.2, S4.1, S4.2, S5.1, S5.2 (merge rules and priority ladder), S6.1, S6.2 (switch modes and units), S7.1, S7.2, S8.1, S9.1, S10.1, S10.2, S10.3, S11.1 (architecture), S11.2 (golden loop), S16.2 (role table), S16.4 (ISO install sequence), S17.4 (overlay), S17.11 (app kinds table), S19.1 (module to options), S22.2 (model diff legend), S22.3 (what a check builds), S22.5, S24.3 (revert moves forward), S26.2 (flake-update branch), S28.1 (secrets layout), S29.1 (cache), S31.1 (scopes), S31.2 (tool tiles), S32.2.

### TTS

- Kokoro-82M (Apache-2.0), run locally. Generate one audio file per scene from the `>` blocks, so a scene can be re-recorded alone and its length drives the edit.
- Apply the pronunciation table above before synthesis.
- Leave about 0.4 s of silence between paragraphs and 1 s between scenes.

### Chapters (for YouTube-style markers)

0 Cold open · 1 The problem · 2 The Nix store · 3 Derivations and nixpkgs · 4 Flakes · 5 Modules · 6 Configuration to system · 7 Generations and rollback · 8 home-manager · 9 Secrets · 10 comin · 11 What nixos-manager is · 12 Installing · 13 First steps · 14 Navigation · 15 Fleet graph · 16 Machines · 17 Machine page · 18 Matrices · 19 Fleet modules · 20 Options · 21 Health · 22 Changes and checks · 23 Trials · 24 Deployments · 25 Dependencies · 26 Inputs · 27 History and Code · 28 Secrets · 29 Binary cache · 30 Homes and upgrades · 31 MCP · 32 Recipes
