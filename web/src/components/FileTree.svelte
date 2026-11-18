<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity'
  import { activePath, app, deletePath, movePath, newFile, newFolder, openFile, openView, refresh } from '../lib/state.svelte'
  import { buildTree, type TreeNode } from '../lib/tree'
  import { dirname } from '../lib/util'
  import Icon from './Icon.svelte'

  const nodes = $derived(buildTree(app.entries, app.status?.files))
  const expanded = new SvelteSet<string>()
  let selected = $state<TreeNode | null>(null)
  let treeEl = $state<HTMLElement>()

  /** Folder that "new file/folder" goes into: the selected folder, or the selected file's folder. */
  const targetFolder = () => (selected ? (selected.dir ? selected.path : dirname(selected.path)) : '')

  function activate(node: TreeNode, e: MouseEvent) {
    selected = node
    if (node.dir) {
      if (expanded.has(node.path)) expanded.delete(node.path)
      else expanded.add(node.path)
    } else if (node.deleted) openView({ kind: 'diff', id: `diff:false:${node.path}`, title: `${node.name} (deleted)`, staged: false, path: node.path })
    else openFile(node.path, e.detail === 0) // keyboard activation moves focus into the editor
  }

  function onKey(e: KeyboardEvent, node: TreeNode) {
    const rows = [...(treeEl?.querySelectorAll<HTMLButtonElement>('button.row-main') ?? [])]
    const i = rows.indexOf(e.currentTarget as HTMLButtonElement)
    const go = (j: number) => rows[Math.max(0, Math.min(rows.length - 1, j))]?.focus()
    if (e.key === 'ArrowDown') go(i + 1)
    else if (e.key === 'ArrowUp') go(i - 1)
    else if (e.key === 'ArrowRight' && node.dir && !expanded.has(node.path)) expanded.add(node.path)
    else if (e.key === 'ArrowLeft' && node.dir && expanded.has(node.path)) expanded.delete(node.path)
    else if (e.key === 'F2') movePath(node.path)
    else if (e.key === 'Delete') deletePath(node.path, node.dir)
    else return
    e.preventDefault()
  }

  const badgeTitle: Record<string, string> = { M: 'Modified', A: 'Added', D: 'Deleted', R: 'Renamed', '?': 'Untracked' }
</script>

<section class="panel" aria-label="Files">
  <div class="panel-head">
    <h2>Files</h2>
    <div class="tools">
      <button class="icon" title="New file" aria-label="New file" onclick={() => newFile(targetFolder())}><Icon name="file-plus" /></button>
      <button class="icon" title="New folder" aria-label="New folder" onclick={() => newFolder(targetFolder())}><Icon name="folder-plus" /></button>
      <button class="icon" title="Refresh" aria-label="Refresh" onclick={refresh}><Icon name="refresh" /></button>
      <button class="icon" title="Collapse all" aria-label="Collapse all" onclick={() => expanded.clear()}><Icon name="collapse" /></button>
    </div>
  </div>

  {#snippet list(items: TreeNode[], depth: number)}
    <ul>
      {#each items as node (node.path)}
        <li>
          <div class="row" class:selected={selected?.path === node.path} class:open={activePath() === node.path}>
            <button
              class="row-main"
              style:padding-left="{8 + depth * 14}px"
              aria-expanded={node.dir ? expanded.has(node.path) : undefined}
              aria-keyshortcuts="F2 Delete"
              title="{node.path}{node.dir ? '' : ' — F2 rename, Delete removes'}"
              onclick={(e) => activate(node, e)}
              onkeydown={(e) => onKey(e, node)}>
              {#if node.dir}
                <Icon name={expanded.has(node.path) ? 'down' : 'right'} size={14} />
              {:else}
                <span class="spacer"></span>
              {/if}
              <span class="name" class:deleted={node.deleted} class:changed={node.dir && node.changed}>{node.name}</span>
              {#if node.badge}<span class="letter b-{node.badge === '?' ? 'u' : node.badge}" title={badgeTitle[node.badge]}>{node.badge}</span>{/if}
            </button>
            {#if !node.deleted}
              <span class="row-tools">
                {#if node.dir}
                  <button class="icon" tabindex="-1" title="New file in {node.name}" aria-label="New file in {node.name}" onclick={() => newFile(node.path)}><Icon name="file-plus" size={14} /></button>
                {/if}
                <button class="icon" tabindex="-1" title="Rename or move (F2)" aria-label="Rename {node.name}" onclick={() => movePath(node.path)}><Icon name="edit" size={14} /></button>
                <button class="icon" tabindex="-1" title="Delete (Del)" aria-label="Delete {node.name}" onclick={() => deletePath(node.path, node.dir)}><Icon name="trash" size={14} /></button>
              </span>
            {/if}
          </div>
          {#if node.dir && expanded.has(node.path)}{@render list(node.children, depth + 1)}{/if}
        </li>
      {/each}
    </ul>
  {/snippet}

  <div class="scroll" bind:this={treeEl}>
    {#if nodes.length}
      {@render list(nodes, 0)}
    {:else}
      <p class="empty">The repository is empty. <button class="link" onclick={() => newFile('')}>Create a file</button></p>
    {/if}
  </div>
</section>

<style>
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .row {
    position: relative;
    display: flex;
    align-items: center;
  }
  .row:hover {
    background: var(--bg-hover);
  }
  .row.open .row-main {
    color: var(--fg-strong);
  }
  .row.selected {
    background: var(--bg-active);
  }
  .row-main {
    display: flex;
    flex: 1;
    align-items: center;
    gap: 4px;
    min-width: 0;
    height: 24px;
    padding-right: 8px;
    color: var(--fg);
    text-align: left;
    background: none;
    border: 0;
    border-radius: 0;
  }
  .row-main:focus-visible {
    outline-offset: -2px;
  }
  .spacer {
    width: 14px;
    flex: none;
  }
  .name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .name.deleted {
    color: var(--fg-muted);
    text-decoration: line-through;
  }
  .name.changed {
    color: var(--yellow);
  }
  .row-tools {
    position: absolute;
    right: 4px;
    display: none;
    gap: 2px;
    background: var(--bg-hover);
  }
  .row:hover .row-tools {
    display: flex;
  }
  .row-tools .icon {
    width: 22px;
    height: 22px;
  }
</style>
