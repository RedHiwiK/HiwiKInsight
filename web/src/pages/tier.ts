// Same definition as the server-side metrics.Tier: tiers by active days in the last 28 days
export function Tier(days28: number): 'heavy' | 'medium' | 'light' | 'once' | 'dormant' {
  if (days28 >= 15) return 'heavy'
  if (days28 >= 5) return 'medium'
  if (days28 >= 2) return 'light'
  if (days28 === 1) return 'once'
  return 'dormant'
}
