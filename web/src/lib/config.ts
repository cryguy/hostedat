/**
 * Returns the instance domain derived from the current hostname.
 * Works for any self-hosted deployment — no hardcoded domain needed.
 */
export function getInstanceDomain(): string {
  return window.location.hostname
}
