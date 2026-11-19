import { describe, expect, it } from 'vitest'
import { categories, recipes } from './catalog'
import { optionSegments, pathName } from './options'

describe('service catalog', () => {
  it('has unique recipes that only set scalar options', () => {
    expect(new Set(recipes.map((r) => r.id)).size).toBe(recipes.length)
    for (const r of recipes) {
      expect(r.settings[0].value).toBe(true)
      for (const s of r.settings) {
        expect(['string', 'number', 'boolean']).toContain(typeof s.value)
        expect(optionSegments(pathName(s.path)).map((x) => x.text)).toEqual(s.path)
      }
    }
  })

  it('lists each category once, in catalog order', () => {
    expect(categories[0]).toBe(recipes[0].category)
    expect(new Set(categories).size).toBe(categories.length)
  })
})
