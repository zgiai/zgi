/** A single primary group per operation. This does not alter remote permissions. */
export function capabilityGroup(id: string): 'messaging' | 'documents' | 'people' | 'resources' {
  const subject = id.split('.')[1];
  if (['document', 'file', 'wiki', 'comment'].includes(subject)) return 'documents';
  if (['contact', 'tenant'].includes(subject)) return 'people';
  if (subject === 'resource') return 'resources';
  return 'messaging';
}
