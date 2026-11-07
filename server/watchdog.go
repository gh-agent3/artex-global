package server

// Intent watchdog: worker-liveness backstop. Resets intents stuck in
// 'running' with no activity back to open so the engine re-claims them. The
// primary fix for the observed freeze is the planner per-round deadline
// (engine.go) — a stuck Plan call froze the planner loop, and with it the
// heartbeat that supervises in-flight workers, so a stalled worker was never
// reaped. This watchdog covers the residual case directly.

import (
	"context"
	"log"
	"time"
)

const (
	watchdogInterval   = 5 * time.Minute
	watchdogMaxIdleMin = 45 // a live worker streams activity; 45min of silence = dead
)

// intentWatchdog resets intents stuck in 'running' with no activity back to
// open so the engine re-claims them.
func (s *Server) intentWatchdog(ctx context.Context) {
	tick := time.NewTicker(watchdogInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.intentWatchdogOnce(ctx)
		}
	}
}

func (s *Server) intentWatchdogOnce(ctx context.Context) {
	if s.m == nil || s.m.pg == nil || s.m.pg.SQLite() {
		return
	}
	ids, err := s.m.pg.StaleRunningIntents(watchdogMaxIdleMin, 10)
	if err != nil {
		log.Printf(logA("[watchdog] stale scan: %v"), err)
		return
	}
	for _, id := range ids {
		reset, err := s.m.pg.ResetRunningIntentToOpen(id)
		if err != nil {
			log.Printf(logA("[watchdog] reset intent %d: %v"), id, err)
			continue
		}
		if !reset {
			continue
		}
		log.Printf(logA("[watchdog] intent %d was running with %d+ min of silence — reset to open for re-claim"), id, watchdogMaxIdleMin)
		// Wake the task so the engine dispatches immediately rather than on the
		// next incidental trigger.
		if exp, err := s.m.pg.ExplorationIDByIntent(id); err == nil {
			if tid, err := s.m.pg.TaskIDByExploration(exp); err == nil {
				if task, ok := s.m.Task(tid); ok {
					task.Notify()
				}
			}
		}
	}
}
