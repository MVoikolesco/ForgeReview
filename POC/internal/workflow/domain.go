// Package workflow contains the generic, provider-independent workflow runtime.
package workflow

import (
	"context"
	"fmt"
)

// AnyContract is the explicit wildcard contract for ports that intentionally
// carry an application-defined value. Concrete contracts must match exactly.
const AnyContract = "any"

type JoinMode string

const (
	JoinAll JoinMode = "all"
	JoinAny JoinMode = "any"
)

type ErrorMode string

const (
	ErrorFail     ErrorMode = "fail"
	ErrorContinue ErrorMode = "continue"
	ErrorRoute    ErrorMode = "route"
)

// PortSpec is declared by a registered node type. Contracts are system-owned
// identifiers; workflow definitions cannot introduce an executor through a port.
type PortSpec struct {
	Key      string
	Contract string
	Required bool
}

// NodeType is a backend-registered capability. The executor is never supplied
// by a workflow definition.
type NodeType struct {
	Key     string
	Inputs  []PortSpec
	Outputs []PortSpec
	Execute Executor
}

// Node configures one instance of a registered NodeType.
type Node struct {
	ID          string
	Type        string
	Config      map[string]any
	Join        JoinMode
	ErrorPolicy ErrorPolicy
}

// ErrorPolicy determines what happens after an executor fails. Route emits a
// sanitized error token through the node type's reserved error output.
type ErrorPolicy struct {
	Mode ErrorMode
}

// Edge connects one declared output port to one declared input port.
type Edge struct {
	ID         string
	FromNodeID string
	FromPort   string
	ToNodeID   string
	ToPort     string
}

// Definition is an immutable workflow graph. MaxSteps is a runtime safety
// limit that also bounds cyclic graphs.
type Definition struct {
	Key      string
	Nodes    []Node
	Edges    []Edge
	MaxSteps int
}

// Scope isolates tokens from independent branches or loop iterations.
type Scope struct {
	ID       string
	ParentID string
}

// Token is an immutable value routed between ports in a single scope.
type Token struct {
	ID       string
	Contract string
	Value    any
	Scope    Scope
}

// Invocation contains only the tokens delivered to the current node and scope.
type Invocation struct {
	Node   Node
	Inputs map[string][]Token
	Scope  Scope
}

// ExecutorResult emits values on declared output ports. The scheduler assigns
// each emitted token its port's registered contract and invocation scope.
type ExecutorResult struct {
	Outputs map[string][]any
}

// Executor is an internal implementation registered with a node type.
type Executor func(context.Context, Invocation) (ExecutorResult, error)

// Registry owns the allow-list of executable node types.
type Registry struct {
	types map[string]NodeType
}

func NewRegistry(types ...NodeType) (*Registry, error) {
	r := &Registry{types: map[string]NodeType{}}
	for _, nodeType := range types {
		if err := r.Register(nodeType); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Registry) Register(nodeType NodeType) error {
	if nodeType.Key == "" || nodeType.Execute == nil {
		return fmt.Errorf("node type requires key and executor")
	}
	if _, exists := r.types[nodeType.Key]; exists {
		return fmt.Errorf("node type %q is already registered", nodeType.Key)
	}
	if err := validatePortSpecs(nodeType.Inputs, nodeType.Outputs); err != nil {
		return fmt.Errorf("node type %q: %w", nodeType.Key, err)
	}
	r.types[nodeType.Key] = nodeType
	return nil
}

func (r *Registry) Type(key string) (NodeType, bool) {
	nodeType, ok := r.types[key]
	return nodeType, ok
}

func validatePortSpecs(inputs, outputs []PortSpec) error {
	for direction, ports := range map[string][]PortSpec{"input": inputs, "output": outputs} {
		seen := map[string]bool{}
		for _, port := range ports {
			if port.Key == "" || port.Contract == "" {
				return fmt.Errorf("%s port requires key and contract", direction)
			}
			if seen[port.Key] {
				return fmt.Errorf("duplicate %s port %q", direction, port.Key)
			}
			seen[port.Key] = true
		}
	}
	return nil
}

// Validate checks graph structure, typed ports, required inputs, and policies
// before a definition can be persisted or scheduled.
func Validate(definition Definition, registry *Registry) error {
	if registry == nil {
		return fmt.Errorf("workflow registry is required")
	}
	if definition.Key == "" {
		return fmt.Errorf("workflow key is required")
	}
	nodes := make(map[string]Node, len(definition.Nodes))
	types := make(map[string]NodeType, len(definition.Nodes))
	for _, node := range definition.Nodes {
		if node.ID == "" {
			return fmt.Errorf("workflow node id is required")
		}
		if _, exists := nodes[node.ID]; exists {
			return fmt.Errorf("duplicate workflow node %q", node.ID)
		}
		nodeType, registered := registry.Type(node.Type)
		if !registered {
			return fmt.Errorf("node %q uses unregistered type %q", node.ID, node.Type)
		}
		join := node.Join
		if join == "" {
			join = JoinAll
		}
		if join != JoinAll && join != JoinAny {
			return fmt.Errorf("node %q has invalid join mode %q", node.ID, node.Join)
		}
		policy := node.ErrorPolicy.Mode
		if policy == "" {
			policy = ErrorFail
		}
		if policy != ErrorFail && policy != ErrorContinue && policy != ErrorRoute {
			return fmt.Errorf("node %q has invalid error policy %q", node.ID, node.ErrorPolicy.Mode)
		}
		if policy == ErrorRoute {
			if _, ok := outputPort(nodeType, "error"); !ok {
				return fmt.Errorf("node %q cannot route errors: type %q has no error output", node.ID, node.Type)
			}
		}
		nodes[node.ID] = node
		types[node.ID] = nodeType
	}
	if len(nodes) == 0 {
		return fmt.Errorf("workflow has no nodes")
	}
	incoming := map[string]map[string]int{}
	edgeIDs := map[string]bool{}
	for _, edge := range definition.Edges {
		if edge.ID == "" {
			return fmt.Errorf("workflow edge id is required")
		}
		if edgeIDs[edge.ID] {
			return fmt.Errorf("duplicate workflow edge %q", edge.ID)
		}
		edgeIDs[edge.ID] = true
		fromType, fromExists := types[edge.FromNodeID]
		toType, toExists := types[edge.ToNodeID]
		if !fromExists || !toExists {
			return fmt.Errorf("edge %q references an unknown node", edge.ID)
		}
		from, fromExists := outputPort(fromType, edge.FromPort)
		to, toExists := inputPort(toType, edge.ToPort)
		if !fromExists || !toExists {
			return fmt.Errorf("edge %q references an unknown port", edge.ID)
		}
		if !contractsCompatible(from.Contract, to.Contract) {
			return fmt.Errorf("edge %q has incompatible contracts %q and %q", edge.ID, from.Contract, to.Contract)
		}
		if incoming[edge.ToNodeID] == nil {
			incoming[edge.ToNodeID] = map[string]int{}
		}
		incoming[edge.ToNodeID][edge.ToPort]++
	}
	for nodeID, nodeType := range types {
		for _, port := range nodeType.Inputs {
			if port.Required && incoming[nodeID][port.Key] == 0 {
				return fmt.Errorf("node %q required input %q is disconnected", nodeID, port.Key)
			}
		}
	}
	return nil
}

func inputPort(nodeType NodeType, key string) (PortSpec, bool) {
	for _, port := range nodeType.Inputs {
		if port.Key == key {
			return port, true
		}
	}
	return PortSpec{}, false
}

func outputPort(nodeType NodeType, key string) (PortSpec, bool) {
	for _, port := range nodeType.Outputs {
		if port.Key == key {
			return port, true
		}
	}
	return PortSpec{}, false
}

func contractsCompatible(from, to string) bool {
	return from == to || to == AnyContract
}
