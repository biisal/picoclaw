package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/commands"
)

func (al *AgentLoop) tryHandleStopCommand(
	ctx context.Context,
	msg bus.InboundMessage,
	sessionKey string,
) bool {
	cmdName, ok := commands.CommandName(msg.Content)
	if !ok || cmdName != "stop" {
		return false
	}

	result, err := al.stopActiveTurnForSession(sessionKey)
	reply := commands.FormatStopReply(result)
	if err != nil {
		reply = "Failed to stop task: " + err.Error()
	}

	if al.channelManager != nil {
		al.channelManager.InvokeTypingStop(msg.Channel, msg.ChatID)
	}
	al.resetMessageToolRound(sessionKey)
	al.PublishResponseIfNeeded(ctx, msg.Channel, msg.ChatID, sessionKey, reply)
	return true
}

func (al *AgentLoop) stopActiveTurnForSession(sessionKey string) (commands.StopResult, error) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return commands.StopResult{}, fmt.Errorf("session key is required")
	}

	result := commands.StopResult{}
	cleared := al.clearSteeringMessagesForScope(sessionKey)
	al.clearPendingSkills(sessionKey)

	ts := al.getActiveTurnState(sessionKey)
	if ts == nil {
		result.Stopped = cleared > 0
		return result, nil
	}

	snap := ts.snapshot()
	result.TaskName = snap.UserMessage

	if strings.HasPrefix(snap.TurnID, pendingTurnPrefix) {
		al.markPendingStop(sessionKey)
		result.Stopped = true
		return result, nil
	}

	if err := al.HardAbort(sessionKey); err != nil {
		if al.getActiveTurnState(sessionKey) == nil {
			result.Stopped = cleared > 0
			return result, nil
		}
		return commands.StopResult{}, err
	}

	result.Stopped = true
	return result, nil
}

func (al *AgentLoop) markPendingStop(sessionKey string) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return
	}
	al.pendingStops.Store(sessionKey, struct{}{})
}

func (al *AgentLoop) takePendingStop(sessionKey string) bool {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return false
	}
	_, ok := al.pendingStops.LoadAndDelete(sessionKey)
	return ok
}

func (al *AgentLoop) resetMessageToolRound(sessionKey string) {
	if strings.TrimSpace(sessionKey) == "" {
		return
	}
	if registry := al.GetRegistry(); registry != nil {
		if agent := registry.GetDefaultAgent(); agent != nil {
			if tool, ok := agent.Tools.Get("message"); ok {
				if resetter, ok := tool.(interface{ ResetSentInRound(sessionKey string) }); ok {
					resetter.ResetSentInRound(sessionKey)
				}
			}
		}
	}
}
