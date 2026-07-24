package taskgraph

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

func newNode(id string, blockedBy ...string) *Node {
	now := time.Now()
	return &Node{
		ID:        id,
		Status:    StatusPending,
		BlockedBy: blockedBy,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// ---------------------------------------------------------------------------
// AddNode / GetNode
// ---------------------------------------------------------------------------

func TestAddNodeAndGetNode(t *testing.T) {
	g := NewGraph()
	n := newNode("a", "b")
	err := g.AddNode(n)
	if err != nil {
		t.Fatalf("unexpected error adding node: %v", err)
	}

	got, ok := g.GetNode("a")
	if !ok {
		t.Fatal("expected to find node 'a'")
	}
	if got.ID != "a" {
		t.Fatalf("expected ID 'a', got %q", got.ID)
	}
	if got.Status != StatusPending {
		t.Fatalf("expected StatusPending, got %q", got.Status)
	}
}

func TestAddNodeDuplicate(t *testing.T) {
	g := NewGraph()
	if err := g.AddNode(newNode("x")); err != nil {
		t.Fatal(err)
	}
	if err := g.AddNode(newNode("x")); err == nil {
		t.Fatal("expected error for duplicate node")
	}
}

func TestGetNodeMissing(t *testing.T) {
	g := NewGraph()
	_, ok := g.GetNode("nonexistent")
	if ok {
		t.Fatal("expected false for missing node")
	}
}

// ---------------------------------------------------------------------------
// AddEdge / Cycle detection
// ---------------------------------------------------------------------------

func TestAddEdge(t *testing.T) {
	g := NewGraph()
	_ = g.AddNode(newNode("a"))
	_ = g.AddNode(newNode("b"))

	if err := g.AddEdge("a", "b"); err != nil {
		t.Fatalf("unexpected error adding edge: %v", err)
	}

	// b should list a in BlockedBy.
	b, _ := g.GetNode("b")
	if len(b.BlockedBy) != 1 || b.BlockedBy[0] != "a" {
		t.Fatalf("expected b.BlockedBy=[\"a\"], got %v", b.BlockedBy)
	}

	// a should list b in Blocks.
	a, _ := g.GetNode("a")
	if len(a.Blocks) != 1 || a.Blocks[0] != "b" {
		t.Fatalf("expected a.Blocks=[\"b\"], got %v", a.Blocks)
	}
}

func TestAddEdgeCycle(t *testing.T) {
	g := NewGraph()
	_ = g.AddNode(newNode("a"))
	_ = g.AddNode(newNode("b"))
	_ = g.AddNode(newNode("c"))
	_ = g.AddEdge("a", "b")
	_ = g.AddEdge("b", "c")

	// Adding c -> a creates a cycle: a -> b -> c -> a.
	err := g.AddEdge("c", "a")
	if err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestAddEdgeCycleSelfLoop(t *testing.T) {
	g := NewGraph()
	_ = g.AddNode(newNode("a"))
	err := g.AddEdge("a", "a")
	if err == nil {
		t.Fatal("expected cycle error for self-loop")
	}
}

func TestAddNodeDetectsCycleViaBlockedBy(t *testing.T) {
	g := NewGraph()
	_ = g.AddNode(newNode("a"))
	_ = g.AddNode(newNode("b"))
	_ = g.AddEdge("a", "b")

	// Adding c with BlockedBy=[b,c] means b->c and c->c (self-cycle).
	c := newNode("c", "b", "c")
	err := g.AddNode(c)
	if err == nil {
		t.Fatal("expected cycle error when BlockedBy contains self-reference")
	}

	_, ok := g.GetNode("c")
	if ok {
		t.Fatal("node 'c' should not exist after failed AddNode")
	}
}

func TestAddNodeBlockedByCycleChain(t *testing.T) {
	g := NewGraph()
	_ = g.AddNode(newNode("a"))
	_ = g.AddNode(newNode("b"))
	_ = g.AddEdge("a", "b")

	// c depends on b, and a depends on c -> cycle: a->b->c->a.
	c := newNode("c", "b")
	_ = g.AddNode(c)
	err := g.AddEdge("c", "a")
	if err == nil {
		t.Fatal("expected cycle error")
	}
}

// ---------------------------------------------------------------------------
// ReadyNodes
// ---------------------------------------------------------------------------

func TestReadyNodes(t *testing.T) {
	g := NewGraph()
	_ = g.AddNode(newNode("a"))           // no deps -> ready
	_ = g.AddNode(newNode("b", "a"))      // depends on a
	_ = g.AddNode(newNode("c", "a"))      // depends on a
	_ = g.AddNode(newNode("d", "b", "c")) // depends on b and c

	ready := g.ReadyNodes()
	if len(ready) != 1 || ready[0].ID != "a" {
		t.Fatalf("expected only 'a' ready, got %v", nodeIDs(ready))
	}

	_ = g.SetStatus("a", StatusCompleted)
	ready = g.ReadyNodes()
	if len(ready) != 2 {
		t.Fatalf("expected 2 ready nodes (b, c), got %v", nodeIDs(ready))
	}
}

func TestReadyNodesBlockedDependency(t *testing.T) {
	g := NewGraph()
	_ = g.AddNode(newNode("a"))
	_ = g.AddNode(newNode("b", "a"))
	_ = g.AddNode(newNode("c", "b"))

	_ = g.SetStatus("a", StatusCompleted)
	// b is still pending, so c should NOT be ready.
	ready := g.ReadyNodes()
	if len(ready) != 1 || ready[0].ID != "b" {
		t.Fatalf("expected only 'b' ready, got %v", nodeIDs(ready))
	}
}

// ---------------------------------------------------------------------------
// DiscoveredDependency
// ---------------------------------------------------------------------------

func TestDiscoveredDependencyBlocksInProgress(t *testing.T) {
	g := NewGraph()
	_ = g.AddNode(newNode("a"))
	_ = g.AddNode(newNode("b"))
	_ = g.SetStatus("b", StatusInProgress)

	err := g.DiscoveredDependency("a", "b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	b, _ := g.GetNode("b")
	if b.Status != StatusBlocked {
		t.Fatalf("expected b to be blocked, got %q", b.Status)
	}
}

func TestDiscoveredDependencyDoesNotBlockCompleted(t *testing.T) {
	g := NewGraph()
	_ = g.AddNode(newNode("a"))
	_ = g.AddNode(newNode("b"))
	_ = g.SetStatus("b", StatusCompleted)

	_ = g.DiscoveredDependency("a", "b")

	b, _ := g.GetNode("b")
	if b.Status != StatusCompleted {
		t.Fatalf("expected b to remain completed, got %q", b.Status)
	}
}

func TestDiscoveredDependencyCreatesCycle(t *testing.T) {
	g := NewGraph()
	_ = g.AddNode(newNode("a"))
	_ = g.AddNode(newNode("b"))
	_ = g.AddEdge("a", "b")

	err := g.DiscoveredDependency("b", "a")
	if err == nil {
		t.Fatal("expected cycle error")
	}
}

// ---------------------------------------------------------------------------
// SetStatus transitions
// ---------------------------------------------------------------------------

func TestSetStatus(t *testing.T) {
	g := NewGraph()
	_ = g.AddNode(newNode("a"))

	transitions := []string{StatusPending, StatusInProgress, StatusCompleted}
	for _, s := range transitions {
		if err := g.SetStatus("a", s); err != nil {
			t.Fatalf("unexpected error setting status %q: %v", s, err)
		}
		n, _ := g.GetNode("a")
		if n.Status != s {
			t.Fatalf("expected status %q, got %q", s, n.Status)
		}
	}
}

func TestSetStatusNonexistent(t *testing.T) {
	g := NewGraph()
	err := g.SetStatus("nope", StatusFailed)
	if err == nil {
		t.Fatal("expected error for nonexistent node")
	}
}

// ---------------------------------------------------------------------------
// Stats
// ---------------------------------------------------------------------------

func TestStats(t *testing.T) {
	g := NewGraph()
	_ = g.AddNode(newNode("a"))
	_ = g.AddNode(newNode("b"))
	_ = g.AddNode(newNode("c"))

	_ = g.SetStatus("a", StatusCompleted)
	_ = g.SetStatus("b", StatusInProgress)

	stats := g.Stats()
	if stats[StatusPending] != 1 {
		t.Fatalf("expected 1 pending, got %d", stats[StatusPending])
	}
	if stats[StatusInProgress] != 1 {
		t.Fatalf("expected 1 in_progress, got %d", stats[StatusInProgress])
	}
	if stats[StatusCompleted] != 1 {
		t.Fatalf("expected 1 completed, got %d", stats[StatusCompleted])
	}
}

func TestStatsEmptyGraph(t *testing.T) {
	g := NewGraph()
	stats := g.Stats()
	if len(stats) != 0 {
		t.Fatalf("expected empty stats, got %v", stats)
	}
}

// ---------------------------------------------------------------------------
// AllNodes (topological ordering)
// ---------------------------------------------------------------------------

func TestAllNodesTopologicalOrder(t *testing.T) {
	g := NewGraph()
	_ = g.AddNode(newNode("a"))
	_ = g.AddNode(newNode("b"))
	_ = g.AddNode(newNode("c"))
	_ = g.AddNode(newNode("d"))
	_ = g.AddEdge("a", "b")
	_ = g.AddEdge("a", "c")
	_ = g.AddEdge("b", "d")
	_ = g.AddEdge("c", "d")

	order := g.AllNodes()
	pos := make(map[string]int)
	for i, n := range order {
		pos[n.ID] = i
	}

	// a must come before b, a before c, and both b and c before d.
	if pos["a"] >= pos["b"] {
		t.Fatalf("expected a before b in topo order")
	}
	if pos["a"] >= pos["c"] {
		t.Fatalf("expected a before c in topo order")
	}
	if pos["b"] >= pos["d"] || pos["c"] >= pos["d"] {
		t.Fatalf("expected b and c before d in topo order")
	}
}

func TestAllNodesDisconnected(t *testing.T) {
	g := NewGraph()
	_ = g.AddNode(newNode("x"))
	_ = g.AddNode(newNode("y"))
	_ = g.AddNode(newNode("z"))

	order := g.AllNodes()
	if len(order) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(order))
	}
}

// ---------------------------------------------------------------------------
// Concurrent access
// ---------------------------------------------------------------------------

func TestConcurrentAccess(t *testing.T) {
	g := NewGraph()

	var wg sync.WaitGroup
	n := 50

	// Concurrently add nodes.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("node_%d", idx)
			_ = g.AddNode(newNode(id))
		}(i)
	}
	wg.Wait()

	// Verify all nodes were added.
	if len(g.AllNodes()) != n {
		t.Fatalf("expected %d nodes, got %d", n, len(g.AllNodes()))
	}

	// Concurrently update status.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("node_%d", idx)
			_ = g.SetStatus(id, StatusCompleted)
		}(i)
	}
	wg.Wait()

	stats := g.Stats()
	if stats[StatusCompleted] != n {
		t.Fatalf("expected %d completed, got %d", n, stats[StatusCompleted])
	}

	// Concurrent reads.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("node_%d", idx)
			g.GetNode(id)
			g.ReadyNodes()
			g.Stats()
		}(i)
	}
	wg.Wait()
}

func TestConcurrentAddEdgeAndDetectCycle(t *testing.T) {
	g := NewGraph()
	_ = g.AddNode(newNode("hub"))
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("leaf_%d", i)
		_ = g.AddNode(newNode(id))
	}

	var wg sync.WaitGroup
	// Concurrently add edges from hub to each leaf.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("leaf_%d", idx)
			_ = g.AddEdge("hub", id)
		}(i)
	}
	wg.Wait()

	order := g.AllNodes()
	pos := make(map[string]int)
	for i, n := range order {
		pos[n.ID] = i
	}

	// All nodes present.
	if len(order) != 21 {
		t.Fatalf("expected 21 nodes, got %d", len(order))
	}
	// hub must be first.
	hubPos := pos["hub"]
	for i := 0; i < 20; i++ {
		leaf := fmt.Sprintf("leaf_%d", i)
		if pos[leaf] <= hubPos {
			t.Fatalf("expected leaf %q after hub", leaf)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func nodeIDs(nodes []*Node) []string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	sort.Strings(ids)
	return ids
}
