<script lang="ts">
  // The console chapter's product shots, drawn in the console's own glyphs: four framed screens with their captions.
  import { frame } from '../lib/ascii'

  const SHOTS = [
    {
      cap: 'fleet · services & programs',
      alt: 'The fleet graph: homeserver, workstation and pi on the left, each linked to the services and programs it runs',
      art: '╭─────╮  ╭──╮\n│ ▣   ├─┬┤≋ │\n╰─────╯ │╰──╯\n╭─────╮ │╭──╮\n│ ▣   ├─┼┤≋ │\n╰─────╯ │╰──╯\n╭─────╮ │╭──╮\n│ ▣   ├─┴┤›_│\n╰─────╯  ╰──╯',
    },
    {
      cap: 'machines / workstation',
      alt: 'A machine page: workstation, with its packages, services, users and the files that configure it',
      art: '█▀▀ █ █ ▀█▀\n● x86_64 ✎\n━━━━──────────\n╭──╮╭──╮╭──╮\n│22││14││ 1│\n╰──╯╰──╯╰──╯\n▌FEATURES\n ░░░ ░░░░ ░░',
    },
    {
      cap: 'changes · before commit',
      alt: "The Changes view: neovim and spotify added to homeserver and workstation, and the diff of each machine's managed.json",
      art: ' [x] managed.json\n [ ] managed.json\n┌──────────────┐\n│+ "packages":[│\n│+   "neovim"  │\n│+ ]           │\n└──────────────┘\n ▐ Commit ▌',
    },
    {
      cap: 'packages · search nixpkgs',
      alt: 'Package search: results from nixpkgs, added to a machine with one click',
      art: 'λ › neo█\n──────────────\n[x] neovim  0.11\n[ ] neofetch 7.1\n[x] neovide 0.15\n░░░░░▒▓▒░░░░░░\n──────────────\n ▐ Add 2 ▌',
    },
  ]
</script>

<div class="showcase">
  {#each SHOTS as s (s.cap)}
    <figure class="shot raised" use:frame={{ shadow: true, foot: s.cap }}>
      <pre class="art" role="img" aria-label={s.alt}>{s.art}</pre>
      <figcaption class="sr-only">{s.cap}</figcaption>
    </figure>
  {/each}
</div>

<style>
  .showcase {
    display: grid;
    grid-template-columns: repeat(2, 34ch);
    gap: calc(var(--lh) * 2) 4ch;
    align-content: start;
  }
  .shot {
    height: calc(var(--lh) * 12);
    margin: 0;
  }
  .art {
    color: var(--accent);
  }
  @media (max-width: 860px) {
    .showcase {
      grid-template-columns: 34ch;
    }
  }
</style>
