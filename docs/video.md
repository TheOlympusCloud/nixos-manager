# Video tutorial

*Nix, NixOS and nixos-manager* (45:53) explains how Nix and NixOS work, then walks through
every part of the console. It is narrated and has chapters.

**Watch the full video:** [One Repo, Every Machine: Nix, NixOS & nixos-manager Explained](https://www.youtube.com/watch?v=6RzpxML3bPw)

Or go straight to a chapter:

| Part | Chapter | Length | What it covers | In the console |
|---|---|---|---|---|
| 0 | [Cold open](https://www.youtube.com/watch?v=6RzpxML3bPw&t=0s) | 1:05 | The problem with machines set up by hand, the NixOS idea, and the map of the video. | Fleet |
| 1 | [The problem Nix solves](https://www.youtube.com/watch?v=6RzpxML3bPw&t=65s) | 1:16 | Why hand-configured machines drift apart, and Nix's answer: every package in its own place. | – |
| 2 | [The Nix store](https://www.youtube.com/watch?v=6RzpxML3bPw&t=141s) | 1:23 | Store paths, what the hash is made of, and closures: the same inputs give the same result anywhere. | Dependencies |
| 3 | [Derivations and nixpkgs](https://www.youtube.com/watch?v=6RzpxML3bPw&t=224s) | 1:47 | Derivations as build recipes, nixpkgs and its branches, and the three shapes of the Nix language. | Packages |
| 4 | [Flakes](https://www.youtube.com/watch?v=6RzpxML3bPw&t=331s) | 1:09 | flake.nix with its inputs and outputs, flake.lock, and pure evaluation. | Inputs |
| 5 | [The module system](https://www.youtube.com/watch?v=6RzpxML3bPw&t=400s) | 1:52 | Options and values, how modules merge, and the priority ladder from mkForce to defaults. | Modules, Options |
| 6 | [From configuration to running system](https://www.youtube.com/watch?v=6RzpxML3bPw&t=512s) | 1:24 | nixosSystem and the toplevel, and what switch, test and boot do to a running machine. | Changes |
| 7 | [Generations, rollback and garbage collection](https://www.youtube.com/watch?v=6RzpxML3bPw&t=596s) | 1:07 | Generations and the boot menu, rolling back, garbage collection and stateVersion. | Deployments |
| 8 | [home-manager](https://www.youtube.com/watch?v=6RzpxML3bPw&t=663s) | 0:36 | Your user's packages and dotfiles in Nix, as a NixOS module or standalone. | Machines › Homes |
| 9 | [Secrets with sops-nix](https://www.youtube.com/watch?v=6RzpxML3bPw&t=699s) | 0:37 | Why the Nix store is no place for secrets, and how sops-nix decrypts them on the machine. | Settings › Secrets |
| 10 | [Deploying a fleet with git and comin](https://www.youtube.com/watch?v=6RzpxML3bPw&t=736s) | 1:30 | Machines that pull: comin deploys every push to main, testing branches, and only moving forward. | Deployments |
| 11 | [What nixos-manager is](https://www.youtube.com/watch?v=6RzpxML3bPw&t=826s) | 1:17 | The server, its working copy and checks, and its rule: no commit or push without a passing check. | The whole console |
| 12 | [Installing the server](https://www.youtube.com/watch?v=6RzpxML3bPw&t=903s) | 1:57 | A NixOS container on Proxmox, the module's options, and Docker with its security trade-off. | – (server setup) |
| 13 | [First steps](https://www.youtube.com/watch?v=6RzpxML3bPw&t=1020s) | 2:18 | The intro and login, the starter fleet, locking the flake and connecting Forgejo. | Settings › Forgejo |
| 14 | [Finding your way around](https://www.youtube.com/watch?v=6RzpxML3bPw&t=1158s) | 1:07 | The navigation rail and its shortcuts, the top bar, branches and the command palette. | Navigation, Ctrl+K |
| 15 | [The fleet graph](https://www.youtube.com/watch?v=6RzpxML3bPw&t=1225s) | 0:43 | Machines, the services and modules they share, and their deployment state on one graph. | Fleet |
| 16 | [Machines and new machines](https://www.youtube.com/watch?v=6RzpxML3bPw&t=1268s) | 2:57 | The Machines view, the New machine wizard, nixos-anywhere and the installer ISO. | Machines |
| 17 | [The machine page](https://www.youtube.com/watch?v=6RzpxML3bPw&t=1445s) | 5:43 | Every tab of a machine's page: overview, essentials, packages (add, remove, replace, customise), dependencies, services, modules, users, homes and their apps, network and storage. | Machines › a machine |
| 18 | [Packages and Services matrices](https://www.youtube.com/watch?v=6RzpxML3bPw&t=1788s) | 0:19 | Every package and service of every machine, side by side. | Packages, Services |
| 19 | [Fleet modules](https://www.youtube.com/watch?v=6RzpxML3bPw&t=1807s) | 1:12 | Bundle packages and settings into a module with options of its own, turned on machine by machine. | Modules |
| 20 | [Options](https://www.youtube.com/watch?v=6RzpxML3bPw&t=1879s) | 0:58 | Browse NixOS options with their real value on each machine, set them with a form, compare and copy. | Options |
| 21 | [Health](https://www.youtube.com/watch?v=6RzpxML3bPw&t=1937s) | 1:06 | What Health flags, and how a Fix button turns a finding into a change to check and push. | Health |
| 22 | [Changes, checks, commits](https://www.youtube.com/watch?v=6RzpxML3bPw&t=2003s) | 2:35 | The per-machine diff in NixOS terms, what a check builds, then commit and push. | Changes |
| 23 | [Trying a change on one machine](https://www.youtube.com/watch?v=6RzpxML3bPw&t=2158s) | 0:51 | Push a checked change to one machine's testing branch, watch it, then promote it or end the trial. | Changes › Try on |
| 24 | [Deployments and rollback](https://www.youtube.com/watch?v=6RzpxML3bPw&t=2209s) | 2:07 | Reports from every machine, watching a rollout, reverting what broke a machine, and notifications. | Deployments |
| 25 | [Dependencies across the fleet](https://www.youtube.com/watch?v=6RzpxML3bPw&t=2336s) | 0:22 | What each machine has installed, with the versions where machines differ. | Dependencies |
| 26 | [Flake inputs and scheduled updates](https://www.youtube.com/watch?v=6RzpxML3bPw&t=2358s) | 0:53 | Input ages, updating one input or all, and a scheduled update you review before it reaches main. | Inputs |
| 27 | [History and Code](https://www.youtube.com/watch?v=6RzpxML3bPw&t=2411s) | 0:44 | Repository and file history with restore, and the editor for hand-written Nix. | History, Code |
| 28 | [Secrets](https://www.youtube.com/watch?v=6RzpxML3bPw&t=2455s) | 1:31 | Where secrets live in the repository, the machines' keys, writing values and using them in options. | Settings › Secrets |
| 29 | [Binary cache](https://www.youtube.com/watch?v=6RzpxML3bPw&t=2546s) | 0:41 | Serving what checks built, so machines download instead of compiling. | – (server option) |
| 30 | [Standalone homes and upgrades](https://www.youtube.com/watch?v=6RzpxML3bPw&t=2587s) | 0:32 | home-manager for computers that aren't NixOS, and upgrading the fleet's managed.nix. | Machines › Homes, Health |
| 31 | [Automation with MCP and API tokens](https://www.youtube.com/watch?v=6RzpxML3bPw&t=2619s) | 1:13 | Token scopes, the MCP endpoint and its tools, under the same rules as the web page. | Settings › API tokens |
| 32 | [Putting it together](https://www.youtube.com/watch?v=6RzpxML3bPw&t=2692s) | 1:01 | Five everyday tasks end to end, from installing a tool to staying up to date. | – |
