package agents

import "fmt"

type Registry struct {
	agents map[string]Agent
}

func NewRegistry() *Registry {
	return &Registry{
		agents: make(map[string]Agent),
	}
}

func (r *Registry) Register(agent Agent) {
	r.agents[agent.Name()] = agent
}

func (r *Registry) Get(name string) (Agent, error) {
	agent, ok := r.agents[name]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", name)
	}

	return agent, nil
}
