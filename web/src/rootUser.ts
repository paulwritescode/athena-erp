// The workspace root account (ADR-0018): one reserved username per
// workspace, matched case-insensitively like the server normalizes it.
// Display only; every refusal is checked again on the server.
export const rootUsername = 'administrator'

export function isRootUsername(username: string | null | undefined): boolean {
  return (username ?? '').trim().toLowerCase() === rootUsername
}
