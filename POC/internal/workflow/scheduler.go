package workflow

import (
	"context"
	"fmt"
)

// Start injects a trusted entry token into a source node output. External
// adapters create starts; they never execute arbitrary workflow code.
type Start struct {
	NodeID string
	Port   string
	Token  Token
}

type NodeExecution struct {
	NodeID string
	Scope  Scope
	Inputs map[string][]Token
	Status string
	Error  string
}

type NodeError struct {
	NodeID  string
	Scope   Scope
	Message string
}

type RunResult struct {
	Executions []NodeExecution
	Errors     []NodeError
}

// Scheduler is a deterministic, token-aware worklist executor.
type Scheduler struct {
	definition Definition
	registry   *Registry
	nodes      map[string]Node
	types      map[string]NodeType
	outgoing   map[string][]Edge
	maxSteps   int
}

func NewScheduler(definition Definition, registry *Registry) (*Scheduler, error) {
	if err := Validate(definition, registry); err != nil {
		return nil, err
	}
	maxSteps := definition.MaxSteps
	if maxSteps < 1 {
		maxSteps = 1000
	}
	scheduler := &Scheduler{definition: definition, registry: registry, nodes: map[string]Node{}, types: map[string]NodeType{}, outgoing: map[string][]Edge{}, maxSteps: maxSteps}
	for _, node := range definition.Nodes {
		scheduler.nodes[node.ID] = node
		scheduler.types[node.ID], _ = registry.Type(node.Type)
	}
	for _, edge := range definition.Edges {
		scheduler.outgoing[edge.FromNodeID+"\x00"+edge.FromPort] = append(scheduler.outgoing[edge.FromNodeID+"\x00"+edge.FromPort], edge)
	}
	return scheduler, nil
}

type scheduledInvocation struct {
	nodeID string
	scope  Scope
	inputs map[string][]Token
}

// Run executes starts in FIFO order. Tokens from different scopes never join.
func (s *Scheduler) Run(ctx context.Context, starts []Start) (RunResult, error) {
	result := RunResult{Executions: []NodeExecution{}, Errors: []NodeError{}}
	buffers := map[string]map[string]map[string][]Token{}
	work := []scheduledInvocation{}
	tokenSequence := 0
	nextToken := func(token Token, contract string, scope Scope) Token {
		tokenSequence++
		if token.ID == "" {
			token.ID = fmt.Sprintf("token-%d", tokenSequence)
		}
		if scope.ID == "" {
			scope.ID = "root"
		}
		token.Contract = contract
		token.Scope = scope
		return token
	}
	var queueReady func(string, Scope)
	queueReady = func(nodeID string, scope Scope) {
		nodeType := s.types[nodeID]
		if len(nodeType.Inputs) == 0 {
			return
		}
		if scope.ID == "" {
			scope.ID = "root"
		}
		if buffers[scope.ID] == nil || buffers[scope.ID][nodeID] == nil {
			return
		}
		join := s.nodes[nodeID].Join
		if join == "" {
			join = JoinAll
		}
		for {
			selected := map[string][]Token{}
			ready := false
			if join == JoinAny {
				for _, port := range nodeType.Inputs {
					if !port.Required || len(buffers[scope.ID][nodeID][port.Key]) == 0 {
						continue
					}
					selected[port.Key] = []Token{buffers[scope.ID][nodeID][port.Key][0]}
					ready = true
					break
				}
			} else {
				ready = true
				for _, port := range nodeType.Inputs {
					if port.Required && len(buffers[scope.ID][nodeID][port.Key]) == 0 {
						ready = false
						break
					}
					if len(buffers[scope.ID][nodeID][port.Key]) > 0 {
						selected[port.Key] = []Token{buffers[scope.ID][nodeID][port.Key][0]}
					}
				}
			}
			if !ready {
				return
			}
			for port := range selected {
				buffers[scope.ID][nodeID][port] = buffers[scope.ID][nodeID][port][1:]
			}
			work = append(work, scheduledInvocation{nodeID: nodeID, scope: scope, inputs: selected})
		}
	}
	var deliver func(Edge, Token) error
	deliver = func(edge Edge, token Token) error {
		targetPort, _ := inputPort(s.types[edge.ToNodeID], edge.ToPort)
		if !contractsCompatible(token.Contract, targetPort.Contract) {
			return fmt.Errorf("token %q contract %q is incompatible with %s.%s", token.ID, token.Contract, edge.ToNodeID, edge.ToPort)
		}
		scopeID := token.Scope.ID
		if scopeID == "" {
			scopeID = "root"
			token.Scope.ID = scopeID
		}
		if buffers[scopeID] == nil {
			buffers[scopeID] = map[string]map[string][]Token{}
		}
		if buffers[scopeID][edge.ToNodeID] == nil {
			buffers[scopeID][edge.ToNodeID] = map[string][]Token{}
		}
		buffers[scopeID][edge.ToNodeID][edge.ToPort] = append(buffers[scopeID][edge.ToNodeID][edge.ToPort], token)
		queueReady(edge.ToNodeID, token.Scope)
		return nil
	}
	route := func(nodeID, port string, token Token) error {
		for _, edge := range s.outgoing[nodeID+"\x00"+port] {
			if err := deliver(edge, token); err != nil {
				return err
			}
		}
		return nil
	}
	for _, start := range starts {
		node, exists := s.nodes[start.NodeID]
		if !exists {
			return result, fmt.Errorf("start references unknown node %q", start.NodeID)
		}
		nodeType := s.types[node.ID]
		if len(nodeType.Inputs) != 0 {
			return result, fmt.Errorf("start node %q must not have input ports", start.NodeID)
		}
		port, exists := outputPort(nodeType, start.Port)
		if !exists {
			return result, fmt.Errorf("start references unknown output %s.%s", start.NodeID, start.Port)
		}
		token := nextToken(start.Token, port.Contract, start.Token.Scope)
		if err := route(start.NodeID, start.Port, token); err != nil {
			return result, err
		}
	}
	for len(work) > 0 {
		if len(result.Executions) >= s.maxSteps {
			return result, fmt.Errorf("workflow scheduler step limit %d exceeded", s.maxSteps)
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		item := work[0]
		work = work[1:]
		node := s.nodes[item.nodeID]
		nodeType := s.types[item.nodeID]
		execution := NodeExecution{NodeID: node.ID, Scope: item.scope, Inputs: item.inputs, Status: "completed"}
		outcome, executionErr := nodeType.Execute(ctx, Invocation{Node: node, Inputs: item.inputs, Scope: item.scope})
		if executionErr != nil {
			execution.Error = executionErr.Error()
			result.Errors = append(result.Errors, NodeError{NodeID: node.ID, Scope: item.scope, Message: executionErr.Error()})
			policy := node.ErrorPolicy.Mode
			if policy == "" {
				policy = ErrorFail
			}
			switch policy {
			case ErrorFail:
				execution.Status = "failed"
				result.Executions = append(result.Executions, execution)
				return result, fmt.Errorf("node %q: %w", node.ID, executionErr)
			case ErrorContinue:
				execution.Status = "continued"
			case ErrorRoute:
				execution.Status = "routed"
				errorPort, _ := outputPort(nodeType, "error")
				token := nextToken(Token{Value: map[string]string{"message": executionErr.Error()}}, errorPort.Contract, item.scope)
				if err := route(node.ID, "error", token); err != nil {
					return result, err
				}
			}
			result.Executions = append(result.Executions, execution)
			continue
		}
		for portKey, values := range outcome.Outputs {
			port, exists := outputPort(nodeType, portKey)
			if !exists {
				return result, fmt.Errorf("node %q emitted undeclared output %q", node.ID, portKey)
			}
			for _, value := range values {
				token := nextToken(Token{Value: value}, port.Contract, item.scope)
				if err := route(node.ID, portKey, token); err != nil {
					return result, err
				}
			}
		}
		result.Executions = append(result.Executions, execution)
	}
	return result, nil
}
