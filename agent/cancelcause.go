package agent

import (
	"context"
	"errors"
	"fmt"
)

// AbortCause names why an agent run's context was cancelled. Every cancellation
// site should attach one so the activity trace can report the real initiator.
type AbortCause struct {
	Code  string
	Short string
	Text  string
}

func (c *AbortCause) Error() string { return c.Text }

func cause(code, short, text string) *AbortCause {
	return &AbortCause{Code: code, Short: short, Text: text}
}

// Causef builds a cause that includes runtime-specific detail.
func Causef(code, short, format string, args ...any) *AbortCause {
	return &AbortCause{Code: code, Short: short, Text: fmt.Sprintf(format, args...)}
}

var (
	// Task-level execution context.
	AbortPausedByUser = cause("paused_by_user", "the user paused the task",
		"The user paused the task through the task control API (POST /api/tasks/{id}/control, action=pause). This Planner/Worker run was cancelled on purpose; a running intent falls back to the frontier (open) and is claimed again from the start when the task resumes")
	AbortPausedByOrchestrator = cause("paused_by_orchestrator", "the orchestrator agent paused the task",
		"The orchestrator agent called the pause_task tool to pause this task. This Planner/Worker run was cancelled on purpose; a running intent falls back to the frontier (open) and runs again after the task resumes")
	AbortTaskDeleted = cause("task_deleted", "the task was deleted",
		"The task is being deleted (DELETE /api/tasks/{id}); the deletion barrier cancelled this task's running Planner, Workers and main agent. The result of this run is no longer used")
	AbortPausedOnReload = cause("paused_on_reload", "paused state restored on startup",
		"On startup the backend restored the task's pause state from the database. This run was cancelled; normally no agent is running during recovery")
	AbortGoalMet = cause("goal_met", "the planner judged the task goal met",
		"The planner judged the task goal met and marked the task done, then cancelled the Workers still running; those intents are marked stopped rather than failed")
	AbortSettleDrainTimeout = cause("settle_drain_timeout", "timeout settlement grace ran out",
		"After the task hit its timeout the engine waited for the running Worker to settle gracefully, but the 90-second drain grace was not enough, so the run was hard-cancelled; intents are marked exhausted and the facts and assets written during settlement are kept")

	// Per-work context.
	AbortKilledByPlanner = cause("killed_by_planner", "the planner killed this intent",
		"The planner called kill_work to stop this intent on purpose, usually because the direction drifted or has no further value; the intent is marked stopped and is not claimed again automatically")
	AbortWorkPausedByUser = cause("work_paused_by_user", "the user paused this worker intent",
		"The user paused the running Worker. This call was cancelled and the intent moved to paused; the registered intent, facts, findings and activity records are all kept, and the intent runs again from the start after resuming")
	AbortWorkCancelledByUser = cause("work_cancelled_by_user", "the user deleted this worker intent",
		"The user deleted the running Worker. This call was cancelled; once the Worker leaves the write area the server applies the deletion mode the user chose - a soft delete only marks the intent deleted and keeps every output, a hard delete cascades the intent and the downstream nodes that only it supported")
	AbortWorkFinished = cause("work_finished", "worker finished and released its context",
		"The Worker finished normally and the engine released its context in detachWork. This is not an interruption; seeing it in an interruption message means the cancellation and the finishing event raced")
	AbortPausedRaceGuard = cause("paused_race_guard", "task paused: no new run started",
		"While the task is paused the engine refuses to hand out a new execution context, which stops a race between claim and pause from starting a Worker; a claimed intent falls back to the frontier")

	// Main Agent and standalone conversation contexts.
	AbortChatStoppedByUser = cause("chat_stopped_by_user", "the user stopped this chat turn",
		"The user pressed stop and cancelled this main-agent or conversation-agent run on purpose. The activity already recorded is kept and the next message can be sent")
	AbortChatPausedWithTask = cause("chat_paused_with_task", "task pause stopped the agent chat",
		"When the user paused the task, the running main-agent conversation was cancelled with it. The activity already recorded is kept; this turn is not replayed automatically after resuming")
	AbortChatTurnFinished = cause("chat_turn_finished", "chat turn finished, context released",
		"This chat turn finished normally and the server is releasing the turn's context. This is not an interruption; seeing it in an interruption message means the cancellation and the finishing event raced")

	// Process-level and per-run hard backstop.
	AbortShutdown = cause("shutdown", "the backend process is shutting down",
		"The backend received SIGINT or SIGTERM and is restarting, updating or shutting down. Every running agent is cancelled; intents left running after the restart are reset to open and run again")
	AbortRunHardTimeout = cause("run_hard_timeout", "hard timeout backstop fired",
		"A single run exceeded the soft wall-clock budget plus the extra grace, meaning a model request or a tool did not return for a long time and the normal round-boundary settlement could not run. Check the last tool call that never returned before the interruption")
)

// AbortReason resolves the named cause attached to a cancelled run context.
func AbortReason(ctx context.Context) (code, short, text string, ok bool) {
	c := context.Cause(ctx)
	if c == nil {
		return "", "", "", false
	}
	var ac *AbortCause
	if errors.As(c, &ac) {
		return ac.Code, ac.Short, ac.Text, true
	}
	switch {
	case errors.Is(c, context.DeadlineExceeded):
		return "deadline_exceeded", "the upstream context reached its deadline",
			"The upstream context reached its deadline, but its owner attached no named cause through WithTimeoutCause: " + c.Error(), true
	case errors.Is(c, context.Canceled):
		return "canceled_no_cause", "the canceller attached no named cause",
			"The upstream context was cancelled, but the canceller attached no named cause through context.WithCancelCause; register one in agent/cancelcause.go and wire it into that cancellation point", true
	default:
		return "other", firstLine(c.Error(), 80), c.Error(), true
	}
}
