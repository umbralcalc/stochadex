package environment

import (
	"github.com/umbralcalc/stochadex/pkg/agent"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// LoadConfigWithAgents fully configures a stochastic process with agent
// interactions included.
type LoadConfigWithAgents struct {
	Settings        *simulator.LoadSettingsConfig
	Implementations *simulator.LoadImplementationsConfig
	Agents          []*agent.AgentConfig
}
