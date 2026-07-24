// Package taskgraph manages a directed acyclic graph of work units for Baton's
// orchestrator. It provides thread-safe operations for adding nodes and edges,
// querying ready work, detecting cycles, and tracking status transitions.
package taskgraph

import (
	"fmt"
	"sync"
	"time"
)

// Status values for nodes.
const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusBlocked    = "blocked"
	StatusSkipped    = "skipped"
)

// Node is a work unit in the task graph.
type Node struct {
	ID           string    `json:"id"`
	Description  string    `json:"description"`
	Issues       []string  `json:"issues"` // issue IDs this work unit addresses
	Status       string    `json:"status"`
	BlockedBy    []string  `json:"blocked_by"` // node IDs this depends on
	Blocks       []string  `json:"blocks"`     // node IDs that depend on this
	SessionID    string    `json:"session_id,omitempty"`
	WorktreePath string    `json:"worktree_path,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Graph is a thread-safe DAG of work units.
type Graph struct {
	mu    sync.RWMutex
	nodes map[string]*Node
	// adj stores the adjacency list for cycle detection: fromID -> set of toIDs.
	adj map[string]map[string]struct{}
}

// NewGraph returns an empty Graph.
func NewGraph() *Graph {
	return &Graph{
		nodes: make(map[string]*Node),
		adj:   make(map[string]map[string]struct{}),
	}
}

// AddNode adds a node to the graph. Returns an error if the node ID already
// exists or if adding it would create a cycle (via pre-populated BlockedBy).
func (g *Graph) AddNode(n *Node) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.nodes[n.ID]; exists {
		return fmt.Errorf("node %q already exists", n.ID)
	}

	// Temporarily insert the node and edges so we can validate the full graph.
	g.nodes[n.ID] = n

	// Add forward edges from each blocked_by dependency.
	for _, depID := range n.BlockedBy {
		if _, ok := g.adj[depID]; !ok {
			g.adj[depID] = make(map[string]struct{})
		}
		g.adj[depID][n.ID] = struct{}{}
	}

	if g.hasCycle() {
		// Roll back.
		for _, depID := range n.BlockedBy {
			delete(g.adj[depID], n.ID)
		}
		delete(g.nodes, n.ID)
		return fmt.Errorf("adding node %q would create a cycle", n.ID)
	}

	// Sync Blocks slices for dependency symmetry.
	for _, depID := range n.BlockedBy {
		if dep, ok := g.nodes[depID]; ok {
			dep.Blocks = append(dep.Blocks, n.ID)
		}
	}

	return nil
}

// AddEdge adds a dependency edge from -> to (to depends on from). Returns an
// error if it would create a cycle.
func (g *Graph) AddEdge(fromID, toID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.nodes[fromID]; !ok {
		return fmt.Errorf("node %q does not exist", fromID)
	}
	if _, ok := g.nodes[toID]; !ok {
		return fmt.Errorf("node %q does not exist", toID)
	}

	// Add edge temporarily.
	if _, ok := g.adj[fromID]; !ok {
		g.adj[fromID] = make(map[string]struct{})
	}
	g.adj[fromID][toID] = struct{}{}

	if g.hasCycle() {
		delete(g.adj[fromID], toID)
		return fmt.Errorf("adding edge %s -> %s would create a cycle", fromID, toID)
	}

	// Update node metadata.
	n := g.nodes[toID]
	n.BlockedBy = append(n.BlockedBy, fromID)
	n.UpdatedAt = time.Now()

	dep := g.nodes[fromID]
	dep.Blocks = append(dep.Blocks, toID)
	dep.UpdatedAt = time.Now()

	return nil
}

// ReadyNodes returns all nodes that have status "pending" and whose BlockedBy
// dependencies are all completed.
func (g *Graph) ReadyNodes() []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var ready []*Node
	for _, n := range g.nodes {
		if n.Status != StatusPending {
			continue
		}
		allDone := true
		for _, depID := range n.BlockedBy {
			dep, ok := g.nodes[depID]
			if !ok || dep.Status != StatusCompleted {
				allDone = false
				break
			}
		}
		if allDone {
			ready = append(ready, n)
		}
	}
	return ready
}

// SetStatus updates a node's status. It returns an error if the node does not
// exist. This operation is thread-safe.
func (g *Graph) SetStatus(id, status string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	n, ok := g.nodes[id]
	if !ok {
		return fmt.Errorf("node %q not found", id)
	}
	n.Status = status
	n.UpdatedAt = time.Now()
	return nil
}

// GetNode returns a node by ID and a boolean indicating whether it was found.
func (g *Graph) GetNode(id string) (*Node, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	n, ok := g.nodes[id]
	return n, ok
}

// AllNodes returns all nodes in topological order. If the graph contains a
// cycle (which should not happen when using AddNode/AddEdge), the partial
// order reachable from non-cyclic sub-graphs is returned.
func (g *Graph) AllNodes() []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Kahn's algorithm for topological sort.
	inDegree := make(map[string]int, len(g.nodes))
	for id := range g.nodes {
		inDegree[id] = 0
	}
	for _, targets := range g.adj {
		for t := range targets {
			inDegree[t]++
		}
	}

	queue := make([]string, 0)
	for id := range inDegree {
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}

	result := make([]*Node, 0, len(g.nodes))
	visited := make(map[string]bool, len(g.nodes))

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if visited[id] {
			continue
		}
		visited[id] = true
		if n, ok := g.nodes[id]; ok {
			result = append(result, n)
		}
		for target := range g.adj[id] {
			inDegree[target]--
			if inDegree[target] == 0 {
				queue = append(queue, target)
			}
		}
	}

	// Append any nodes not reached by Kahn's (cyclic leftovers).
	for _, n := range g.nodes {
		if !visited[n.ID] {
			result = append(result, n)
		}
	}

	return result
}

// Stats returns counts of nodes grouped by status.
func (g *Graph) Stats() map[string]int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	stats := make(map[string]int)
	for _, n := range g.nodes {
		stats[n.Status]++
	}
	return stats
}

// DiscoveredDependency records a dependency not declared upfront. It adds the
// edge and, if the dependent node (toID) is currently in progress, marks it as
// blocked. Returns an error if the edge would create a cycle.
func (g *Graph) DiscoveredDependency(fromID, toID string) error {
	if err := g.AddEdge(fromID, toID); err != nil {
		return err
	}

	// Check if the dependent (toID) is in progress and should become blocked.
	g.mu.Lock()
	if n, ok := g.nodes[toID]; ok && n.Status == StatusInProgress {
		n.Status = StatusBlocked
		n.UpdatedAt = time.Now()
	}
	g.mu.Unlock()

	return nil
}

// hasCycle performs a DFS-based cycle detection. Must be called with g.mu held
// (or at least a write lock).
func (g *Graph) hasCycle() bool {
	white := make(map[string]bool) // unvisited
	grey := make(map[string]bool)  // in current DFS stack (being explored)
	black := make(map[string]bool) // fully explored

	for id := range g.nodes {
		white[id] = true
	}

	for id := range white {
		if g.dfsCycle(id, white, grey, black) {
			return true
		}
	}
	return false
}

// dfsCycle is the recursive step of the cycle detection. Returns true if a
// cycle is found.
func (g *Graph) dfsCycle(id string, white, grey, black map[string]bool) bool {
	delete(white, id)
	grey[id] = true

	for target := range g.adj[id] {
		if black[target] {
			continue
		}
		if grey[target] {
			// Found a back-edge: cycle detected.
			return true
		}
		if g.dfsCycle(target, white, grey, black) {
			return true
		}
	}

	delete(grey, id)
	black[id] = true
	return false
}
