// Dialogs opened from anywhere: package search.
export const pkgSearch = $state({ open: false, query: '', hosts: [] as string[] })

/** Open the package search, prefilled, with these machines picked as targets. */
export function openPackageSearch(query = '', hosts: string[] = []) {
  Object.assign(pkgSearch, { open: true, query, hosts })
}
