package loop

import (
	"context"
)

// ReActState define a interface para o State Pattern da máquina de estados do Agente
type ReActState interface {
	Execute(ctx context.Context, al *AgenticLoop, iter *IterationContext) (ReActState, error)
	Name() string
}

// IterationContext encapsula o estado transitório de uma iteração no AgenticLoop
type IterationContext struct {
	Iteration           int
	ConsecutiveFailures int
	HasFailure          bool
	TimerScheduled      bool
	Intent              string
}
