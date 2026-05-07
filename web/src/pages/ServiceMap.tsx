import { useEffect, useState, useCallback, useRef } from 'react'
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

// ─────────────────── Custom Node ───────────────────

type ServiceNodePayload = {
  label: string
  spanCount: number
  errorRate: number
  p99Ms: number
  reqPerSec: number
}

function ServiceNodeComponent({ data, selected }: NodeProps<Node<ServiceNodePayload>>) {
  const radius = Math.max(24, Math.min(50, 20 + Math.log1p(data.spanCount) * 5))
  // HSL: 120° (green) when errorRate=0, 0° (red) when errorRate=1
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

// ─────────────────── Custom Edge ───────────────────

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

// ─────────────────── Dagre layout ───────────────────

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

// ─────────────────── Page ───────────────────

export function ServiceMapPage() {
  const [graph, setGraph] = useState<DependencyGraph | null>(null)
  const [nodes, setNodes, onNodesChange] = useNodesState<Node<ServiceNodePayload>>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])
  const [selectedNode, setSelectedNode] = useState<ServiceNodeData | null>(null)
  const sidebarRef = useRef<HTMLDivElement>(null)

  const loadGraph = useCallback(async () => {
    try {
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
      console.error(err)
    }
  }, [setNodes, setEdges])

  useEffect(() => {
    void loadGraph()
    const interval = setInterval(() => { void loadGraph() }, 30000)
    return () => clearInterval(interval)
  }, [loadGraph])

  if (!graph || graph.services.length === 0) {
    return (
      <div className="p-8 text-center text-muted-foreground">
        No service dependencies yet. Waiting for traces&hellip;
      </div>
    )
  }

  const handleNodeClick = (_: React.MouseEvent, node: Node) => {
    const svc = graph.services.find((s: ServiceNodeData) => s.name === node.id)
    setSelectedNode(svc ?? null)
  }

  return (
    <div className="flex h-[calc(100vh-64px)]">
      <div className="flex-1 border rounded-lg overflow-hidden">
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

      {selectedNode && (
        <div ref={sidebarRef} className="w-72 border-l p-4 overflow-y-auto bg-background">
          <div className="flex items-center justify-between mb-3">
            <h3
              className="font-semibold text-sm"
              style={{ color: getServiceColor(selectedNode.name) }}
            >
              {selectedNode.name}
            </h3>
            <button className="text-muted-foreground hover:text-foreground text-xs" onClick={() => setSelectedNode(null)}>
              ✕
            </button>
          </div>
          <div className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-muted-foreground">Spans</span>
              <span className="font-mono">{selectedNode.spanCount.toLocaleString()}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Error rate</span>
              <span className={`font-mono ${selectedNode.errorRate > 0.05 ? 'text-red-600' : ''}`}>
                {(selectedNode.errorRate * 100).toFixed(1)}%
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">P99 latency</span>
              <span className="font-mono">{selectedNode.p99Ms.toFixed(1)}ms</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Req/s</span>
              <span className="font-mono">{selectedNode.reqPerSec.toFixed(2)}</span>
            </div>
          </div>
          <div className="mt-4">
            <h4 className="text-xs font-semibold text-muted-foreground uppercase mb-2">Edges</h4>
            {graph.edges
              .filter((e: ServiceEdge) => e.caller === selectedNode.name || e.callee === selectedNode.name)
              .map((e: ServiceEdge) => (
                <div key={`${e.caller}-${e.callee}`} className="text-xs border rounded p-2 mb-1">
                  <div className="font-mono">{e.caller} → {e.callee}</div>
                  <div className="text-muted-foreground">{e.count} calls · P99 {e.p99Ms.toFixed(0)}ms</div>
                </div>
              ))}
          </div>
        </div>
      )}
    </div>
  )
}
