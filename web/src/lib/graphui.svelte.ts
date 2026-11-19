// Hover and selection in the fleet graph, read by the node components.
export const graphUi = $state({
  hover: '',
  selected: '',
  /** Nodes related to the hovered or selected one; null when nothing is focused. */
  lit: null as Set<string> | null,
  /** Packages groups expanded in the graph (machine names). */
  expanded: [] as string[],
})

export function toggleExpanded(host: string) {
  const i = graphUi.expanded.indexOf(host)
  if (i >= 0) graphUi.expanded.splice(i, 1)
  else graphUi.expanded.push(host)
}
