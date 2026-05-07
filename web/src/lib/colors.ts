const SERVICE_COLORS = [
  '#2563eb', '#059669', '#d97706', '#7c3aed', '#db2777',
  '#0891b2', '#65a30d', '#dc2626', '#4f46e5', '#0d9488',
]

const serviceColorMap = new Map<string, string>()
let colorIndex = 0

export function getServiceColor(service: string): string {
  if (!serviceColorMap.has(service)) {
    serviceColorMap.set(service, SERVICE_COLORS[colorIndex % SERVICE_COLORS.length])
    colorIndex++
  }
  return serviceColorMap.get(service)!
}

export function getKindLabel(kind: number): string {
  switch (kind) {
    case 1: return 'INTERNAL'
    case 2: return 'SERVER'
    case 3: return 'CLIENT'
    case 4: return 'PRODUCER'
    case 5: return 'CONSUMER'
    default: return 'UNKNOWN'
  }
}

export function getStatusLabel(code: number): string {
  switch (code) {
    case 0: return 'UNSET'
    case 1: return 'OK'
    case 2: return 'ERROR'
    default: return 'UNKNOWN'
  }
}
