import { useEffect, useState, useCallback, useRef } from 'react'
import { GitBranchPlus, Network, Telescope } from 'lucide-react'
import {
  ReactFlow,
  Background,
  Controls,
  Handle,
  Position,
  BaseEdge,
  EdgeLabelRenderer,
  getStraightPath,
  useNodesState,
  useEdgesState,
} from '@xyflow/react'
import type { Node, Edge, NodeProps, EdgeProps } from '@xyflow/react'
import dagre from '@dagrejs/dagre'
import '@xyflow/react/dist/style.css'
import { api } from '@/api/client'
import type { DependencyGraph, ServiceNode as ServiceNodeData, ServiceEdge } from '@/types'
import { getServiceColor } from '@/lib/colors'
import { PageState } from '@/components/ui/page-state'
import { getErrorMessage } from '@/lib/errors'

type ServiceNodePayload = {
  label: string
  spanCount: number
  errorRate: number
  p99Ms: number
  reqPerSec: number
}

function ServiceNodeComponent({ data, selected }: NodeProps<Node<ServiceNodePayload>>) {
  const radius = Math.max(24, Math.min(50, 20 + Math.log1p(data.spanCount) * 5))
  const hue = Math.round((1 - Math.min(data.errorRate, 1)) * 120)
  const fill = `hsl(${hue}, 70%, 45%)`

  return (
    <div
      style={{
        width: radius * 2,
        height: radius * 2,
        borderRadius: '50%',
        background: fill,
        border: selected ? '3px solid white' : '2px solid rgba(255,255,255,0.4)',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        cursor: 'pointer',
        boxShadow: '0 2px 8px rgba(0,0,0,0.25)',
        transition: 'transform 0.15s',
      }}
    >
      <span style={{ color: 'white', fontWeight: 600, fontSize: 11, textAlign: 'center', lineHeight: 1.2, padding: '0 4px', wordBreak: 'break-word' }}>
        {data.label}
      </span>
      {data.errorRate > 0.01 && (
        <span style={{ color: '#fca5a5', fontSize: 9 }}>{(data.errorRate * 100).toFixed(0)}% err</span>
      )}
      <Handle type="target" position={Position.Left} style={{ opacity: 0 }} />
      <Handle type="source" position={Position.Right} style={{ opacity: 0 }} />
    </div>
  )
}

function DependencyEdgeComponent({
  id, sourceX, sourceY, targetX, targetY, data,
}: EdgeProps<Edge<{ count: number; p99Ms: number; high: boolean }>>) {
  const [edgePath, labelX, labelY] = getStraightPath({ sourceX, sourceY, targetX, targetY })
  const strokeWidth = Math.max(1, 1 + Math.log1p((data?.count ?? 0)))

  return (
    <>
      <BaseEdge id={id} path={edgePath} style={{ strokeWidth, stroke: data?.high ? '#f59e0b' : '#64748b' }} />
      {data?.high && (
        <circle r={4} fill="#f59e0b">
          <animateMotion dur="1.5s" repeatCount="indefinite" path={edgePath} />
        </circle>
      )}
      <EdgeLabelRenderer>
        <div
          style={{
            position: 'absolute',
            transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
            fontSize: 10,
            background: 'rgba(255,255,255,0.9)',
            padding: '1px 4px',
            borderRadius: 4,
            pointerEvents: 'none',
            color: '#374151',
            fontFamily: 'monospace',
          }}
          className="nodrag nopan"
        >
          {data?.p99Ms != null ? `P99 ${data.p99Ms.toFixed(0)}ms` : ''}
        </div>
      </EdgeLabelRenderer>
    </>
  )
}

const nodeTypes = { serviceNode: ServiceNodeComponent }
const edgeTypes = { dependencyEdge: DependencyEdgeComponent }

function applyDagreLayout(
  nodes: Node<ServiceNodePayload>[],
  edges: Edge[]
): Node<ServiceNodePayload>[] {
  const g = new dagre.graphlib.Graph()
  g.setGraph({ rankdir: 'LR', nodesep: 80, ranksep: 120 })
  g.setDefaultEdgeLabel(() => ({}))

  for (const node of nodes) {
    const radius = Math.max(24, Math.min(50, 20 + Math.log1p(node.data.spanCount) * 5))
    g.setNode(node.id, { width: radius * 2 + 20, height: radius * 2 + 20 })
  }
  for (const edge of edges) {
    g.setEdge(edge.source, edge.target)
  }
  dagre.layout(g)

  return nodes.map(node => {
    const { x, y } = g.node(node.id)
    return { ...node, position: { x, y } }
  })
}

export function ServiceMapPage() {
  const [graph, setGraph] = useState<DependencyGraph | null>(null)
  const [nodes, setNodes, onNodesChange] = useNodesState<Node<ServiceNodePayload>>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])
  const [selectedNode, setSelectedNode] = useState<ServiceNodeData | null>(null)
  const sidebarRef = useRef<HTMLDivElement>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const loadGraph = useCallback(async () => {
    try {
      setError(null)
      const g = await api.getDependencies()
      setGraph(g)

      const maxCount = Math.max(...(g.edges.map((e: ServiceEdge) => e.count)), 1)
      const avgCount = g.edges.reduce((s: number, e: ServiceEdge) => s + e.count, 0) / Math.max(g.edges.length, 1)

      const rawNodes: Node<ServiceNodePayload>[] = g.services.map((s: ServiceNodeData) => ({
        id: s.name,
        type: 'serviceNode',
        position: { x: 0, y: 0 },
        data: {
          label: s.name,
          spanCount: s.spanCount,
          errorRate: s.errorRate,
          p99Ms: s.p99Ms,
          reqPerSec: s.reqPerSec,
        },
      }))

      const rawEdges: Edge[] = g.edges.map((e: ServiceEdge) => ({
        id: `${e.caller}-${e.callee}`,
        source: e.caller,
        target: e.callee,
        type: 'dependencyEdge',
        data: {
          count: e.count,
          p99Ms: e.p99Ms,
          high: e.count > avgCount * 2 || e.count === maxCount,
        },
      }))

      const laid = applyDagreLayout(rawNodes, rawEdges)
      setNodes(laid)
      setEdges(rawEdges)
    } catch (err) {
      setError(getErrorMessage(err, 'Failed to load service dependency graph.'))
    } finally {
      setLoading(false)
    }
  }, [setNodes, setEdges])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadGraph()
    }, 0)
    const interval = setInterval(() => { void loadGraph() }, 30000)
    return () => {
      window.clearTimeout(timer)
      clearInterval(interval)
    }
  }, [loadGraph])

  if (loading && !graph) {
    return <PageState title="Loading service map" description="Fetching dependency graph and laying out services." />
  }

  if (error && !graph) {
    return <PageState title="Unable to load service map" description={error} actionLabel="Retry" onAction={() => { void loadGraph() }} />
  }

  if (!graph || graph.services.length === 0) {
    return (
      <PageState title="No dependencies yet" description="Ingest traces with cross-service calls to populate the service map." />
    )
  }

  const handleNodeClick = (_: React.MouseEvent, node: Node) => {
    const svc = graph.services.find((s: ServiceNodeData) => s.name === node.id)
    setSelectedNode(svc ?? null)
  }

  return (
    <div className="space-y-5">
      <section className="relative overflow-hidden rounded-[32px] border border-border/70 bg-card/92 p-6 shadow-[0_30px_110px_-52px_rgba(15,23,42,0.55)] backdrop-blur sm:p-8">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_left,_rgba(14,165,233,0.14),_transparent_42%)]" />
        <div className="relative grid gap-5 lg:grid-cols-[minmax(0,1.5fr)_minmax(320px,0.9fr)]">
          <div className="space-y-3">
            <div className="inline-flex items-center gap-2 rounded-full border border-border/70 bg-background/70 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">
              <Network className="h-3.5 w-3.5" />
              Dependency topology
            </div>
            <h1 className="text-4xl font-semibold tracking-tight text-foreground sm:text-5xl">
              Follow cross-service pressure, saturation, and failure propagation.
            </h1>
            <p className="max-w-2xl text-sm leading-6 text-muted-foreground sm:text-base">
              Node size tracks span volume, color shifts with error rate, and animated edges call out the busiest
              dependencies so you can reason about topology before drilling into a trace.
            </p>
          </div>

          <div className="grid gap-3 sm:grid-cols-3 lg:grid-cols-1">
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                <GitBranchPlus className="h-3.5 w-3.5" />
                Services
              </div>
              <div className="mt-3 text-3xl font-semibold text-foreground">{graph.services.length}</div>
              <div className="mt-1 text-xs text-muted-foreground">Distinct services included in the dependency graph</div>
            </div>
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                <Telescope className="h-3.5 w-3.5" />
                Edges
              </div>
              <div className="mt-3 text-3xl font-semibold text-foreground">{graph.edges.length}</div>
              <div className="mt-1 text-xs text-muted-foreground">Highlighted edges indicate the busiest call paths</div>
            </div>
            <div className="rounded-[24px] border border-border/70 bg-background/70 p-4">
              <div className="text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">Operator hint</div>
              <div className="mt-3 text-sm font-medium text-foreground">Click a node to pin service details and edge metrics.</div>
            </div>
          </div>
        </div>
      </section>

      <section className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="overflow-hidden rounded-[28px] border border-border/70 bg-card/88 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border/70 px-5 py-4">
            <div>
              <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">Graph canvas</div>
              <div className="mt-1 text-sm text-muted-foreground">Drag, pan, and zoom to isolate service clusters and high-volume paths.</div>
            </div>
            <span className="rounded-full border border-border/70 bg-background/70 px-3 py-1.5 text-sm text-muted-foreground">
              {graph.services.length} services / {graph.edges.length} dependencies
            </span>
          </div>
          <div className="h-[calc(100vh-320px)] min-h-[560px]">
            <ReactFlow
              nodes={nodes}
              edges={edges}
              onNodesChange={onNodesChange}
              onEdgesChange={onEdgesChange}
              nodeTypes={nodeTypes}
              edgeTypes={edgeTypes}
              onNodeClick={handleNodeClick}
              fitView
            >
              <Background />
              <Controls />
            </ReactFlow>
          </div>
        </div>

        <aside ref={sidebarRef} className="rounded-[28px] border border-border/70 bg-card/88 p-5 shadow-[0_24px_90px_-52px_rgba(15,23,42,0.45)] backdrop-blur">
          {selectedNode ? (
            <>
              <div className="mb-4 flex items-center justify-between">
                <div>
                  <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">Selected service</div>
                  <h3
                    className="mt-1 text-xl font-semibold tracking-tight"
                    style={{ color: getServiceColor(selectedNode.name) }}
                  >
                    {selectedNode.name}
                  </h3>
                </div>
                <button type="button" aria-label="Close service details" className="rounded-full border border-border/70 px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground" onClick={() => setSelectedNode(null)}>
                  Close
                </button>
              </div>
              <div className="space-y-2 text-sm">
                <div className="flex justify-between rounded-2xl border border-border/70 bg-background/60 px-3 py-2">
                  <span className="text-muted-foreground">Spans</span>
                  <span className="font-mono">{selectedNode.spanCount.toLocaleString()}</span>
                </div>
                <div className="flex justify-between rounded-2xl border border-border/70 bg-background/60 px-3 py-2">
                  <span className="text-muted-foreground">Error rate</span>
                  <span className={`font-mono ${selectedNode.errorRate > 0.05 ? 'text-red-600' : ''}`}>
                    {(selectedNode.errorRate * 100).toFixed(1)}%
                  </span>
                </div>
                <div className="flex justify-between rounded-2xl border border-border/70 bg-background/60 px-3 py-2">
                  <span className="text-muted-foreground">P99 latency</span>
                  <span className="font-mono">{selectedNode.p99Ms.toFixed(1)}ms</span>
                </div>
                <div className="flex justify-between rounded-2xl border border-border/70 bg-background/60 px-3 py-2">
                  <span className="text-muted-foreground">Req/s</span>
                  <span className="font-mono">{selectedNode.reqPerSec.toFixed(2)}</span>
                </div>
              </div>

              <div className="mt-5">
                <h4 className="mb-2 text-xs font-semibold uppercase tracking-[0.22em] text-muted-foreground">Connected edges</h4>
                {graph.edges
                  .filter((e: ServiceEdge) => e.caller === selectedNode.name || e.callee === selectedNode.name)
                  .map((e: ServiceEdge) => (
                    <div key={`${e.caller}-${e.callee}`} className="mb-2 rounded-2xl border border-border/70 bg-background/60 p-3 text-xs">
                      <div className="font-mono text-foreground">{e.caller} → {e.callee}</div>
                      <div className="mt-1 text-muted-foreground">{e.count} calls · P99 {e.p99Ms.toFixed(0)}ms</div>
                    </div>
                  ))}
              </div>
            </>
          ) : (
            <div className="flex h-full min-h-72 flex-col justify-center text-center">
              <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl border border-border/70 bg-background/70 text-primary">
                <Network className="h-6 w-6" />
              </div>
              <h3 className="mt-4 text-2xl font-semibold tracking-tight text-foreground">Select a service node</h3>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">
                Click any service bubble in the graph to inspect span volume, error rate, p99 latency, and the
                connected dependencies around that node.
              </p>
            </div>
          )}
        </aside>
      </section>
    </div>
  )
}
