<script lang="ts">
  import { api, type Host } from '../../lib/api'
  import { secretNameProblem } from '../../lib/secrets'
  import { refresh } from '../../lib/state.svelte'
  import { report, toast } from '../../lib/ui.svelte'

  /**
   * One of the machine's secrets, by name, or a new one: its value goes, encrypted,
   * into the machine's secrets (Settings → Secrets), and only its name comes back.
   */
  let { host, value = '', label, onpick }: { host: Host; value?: string; label: string; onpick: (name: string) => void } = $props()
  const names = $derived((host.secrets ?? []).map((s) => s.name))
  let adding = $state(false)
  let name = $state('')
  let secret = $state('')
  let busy = $state(false)
  const problem = $derived(secretNameProblem(name.trim()))

  function choose(e: Event & { currentTarget: HTMLSelectElement }) {
    const v = e.currentTarget.value
    if (v === '+') {
      adding = true
      e.currentTarget.value = value
    } else if (v) onpick(v)
  }
  async function create(e: SubmitEvent) {
    e.preventDefault()
    const n = name.trim()
    if (!n || problem || !secret) return
    busy = true
    try {
      await api.setSecret(host.name, n, secret)
      toast(`Saved ${n}, encrypted, for ${host.name}`, 'ok')
      await refresh()
      adding = false
      name = secret = ''
      onpick(n)
    } catch (err) {
      report(err, `Could not save ${n}`)
    } finally {
      busy = false
    }
  }
</script>

<span class="pick">
  <select class="short mono" aria-label={label} {value} onchange={choose}>
    {#if !value}<option value="">Pick a secret…</option>{/if}
    {#if value && !names.includes(value)}<option value={value}>{value} (not on {host.name} yet)</option>{/if}
    {#each names as n (n)}<option value={n}>{n}</option>{/each}
    <option value="+">New secret…</option>
  </select>
  {#if adding}
    <form class="new" onsubmit={create}>
      <input class="mono short" placeholder="music-password" aria-label="Name of the new secret" bind:value={name} spellcheck="false" autocomplete="off" aria-invalid={!!problem} />
      <input type="password" class="short" placeholder="Value" aria-label="Its value" bind:value={secret} autocomplete="new-password" />
      <button type="submit" class="small primary" disabled={busy || !name.trim() || !!problem || !secret}>{busy ? 'Saving…' : 'Save for ' + host.name}</button>
      <button type="button" class="small ghost" onclick={() => (adding = false)}>Cancel</button>
      {#if problem}<span class="error-text small">{problem}</span>{/if}
    </form>
  {/if}
</span>

<style>
  .pick {
    display: inline-flex;
    flex-wrap: wrap;
    gap: 1ch;
    align-items: baseline;
  }
  .new {
    display: flex;
    flex-wrap: wrap;
    gap: 1ch;
    align-items: baseline;
  }
</style>
