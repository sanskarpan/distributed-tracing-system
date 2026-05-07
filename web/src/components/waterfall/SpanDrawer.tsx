import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Badge } from '@/components/ui/badge'
import type { SpanDetailDTO } from '@/types'
import { getKindLabel, getStatusLabel, getServiceColor } from '@/lib/colors'

interface Props {
  span: SpanDetailDTO | null
  open: boolean
  onClose: () => void
  onParentClick?: (parentSpanId: string) => void
  allSpans?: SpanDetailDTO[]
}

export function SpanDrawer({ span, open, onClose, onParentClick, allSpans }: Props) {
  if (!span) return null
  const durationMs = span.durationMs
  const isError = span.status.code === 2
  const parentSpan = span.parentSpanId
    ? allSpans?.find(s => s.spanId === span.parentSpanId)
    : undefined

  return (
    <Sheet open={open} onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="w-[480px] overflow-y-auto">
        <SheetHeader>
          <SheetTitle className="text-sm font-mono">{span.name}</SheetTitle>
          <div className="flex gap-2 flex-wrap">
            <Badge style={{ backgroundColor: getServiceColor(span.serviceName) }} className="text-white">
              {span.serviceName}
            </Badge>
            <Badge variant={isError ? 'destructive' : 'secondary'}>
              {getStatusLabel(span.status.code)}
            </Badge>
            <Badge variant="outline">{getKindLabel(span.kind)}</Badge>
          </div>
        </SheetHeader>

        <div className="mt-4 space-y-4">
          {/* Parent link */}
          {parentSpan && onParentClick && (
            <button
              className="text-xs text-blue-600 hover:underline text-left"
              onClick={() => onParentClick(parentSpan.spanId)}
            >
              ↑ parent: {parentSpan.name}
            </button>
          )}

          {/* Stats */}
          <div className="grid grid-cols-2 gap-2 text-sm">
            <div>
              <span className="text-muted-foreground">Duration: </span>
              <span className="font-mono">{durationMs.toFixed(2)}ms</span>
            </div>
            <div>
              <span className="text-muted-foreground">Depth: </span>
              <span>{span.depth}</span>
            </div>
          </div>

          {span.status.message && (
            <div className="text-sm text-red-600 bg-red-50 p-2 rounded">
              {span.status.message}
            </div>
          )}

          {/* Attributes */}
          {span.attributes.length > 0 && (
            <div>
              <h4 className="text-sm font-semibold mb-2">Attributes</h4>
              <table className="w-full text-xs">
                <thead>
                  <tr className="text-muted-foreground">
                    <th className="text-left p-1">Key</th>
                    <th className="text-left p-1">Value</th>
                  </tr>
                </thead>
                <tbody>
                  {span.attributes.map((attr, i) => (
                    <tr key={i} className="border-t">
                      <td className="p-1 font-mono">{attr.key}</td>
                      <td className="p-1 font-mono break-all">
                        {attr.stringValue ??
                          attr.intValue ??
                          attr.doubleValue ??
                          String(attr.boolValue ?? '')}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {/* Events */}
          {span.events.length > 0 && (
            <div>
              <h4 className="text-sm font-semibold mb-2">Events</h4>
              <div className="space-y-1">
                {span.events.map((e, i) => (
                  <div key={i} className="text-xs border rounded p-2">
                    <span className="font-mono text-muted-foreground">
                      +{((e.timeUnixNano - span.startTimeUnixNano) / 1e6).toFixed(1)}ms
                    </span>
                    <span className="ml-2 font-semibold">{e.name}</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Links */}
          {span.links.length > 0 && (
            <div>
              <h4 className="text-sm font-semibold mb-2">Links</h4>
              <div className="space-y-1">
                {span.links.map((link, i) => (
                  <div key={i} className="text-xs border rounded p-2 font-mono">
                    <div>Trace: {link.traceId}</div>
                    <div>Span: {link.spanId}</div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
