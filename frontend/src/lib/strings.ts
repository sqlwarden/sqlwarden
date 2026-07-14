export function slugify(value: string, options: { maxLength?: number } = {}) {
  const slug = value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')

  return options.maxLength === undefined ? slug : slug.slice(0, options.maxLength)
}
