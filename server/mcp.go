package server

// MCP ingress: ARTEX as a stateless MCP tool server for external harnesses
// (paperclip and friends). Transport is the Streamable HTTP subset implemented by
// the mcpserver package: POST /mcp with JSON-RPC, Bearer token auth.
//
// Security posture (fail closed):
//   - ARTEX_MCP_TOKEN unset -> the endpoint is not mounted at all (404), so an
//     unattended instance never exposes tools;
//   - wrong token -> 401;
//   - the tool surface is fixed and narrow: task intake + status. No shell, no
//     file writes, no LLM config, no intercept rules.
//
// Stateless: the mcpserver.Server is rebuilt per request, so tool visibility
// always reflects the current process state.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"artex-global/db"
	"artex-global/mcpserver"
)

// mcpToken returns the bearer token that unlocks /mcp, or empty = disabled.
func mcpToken() string {
	return strings.TrimSpace(os.Getenv("ARTEX_MCP_TOKEN"))
}

// mcpHandler mounts the MCP ingress. Token empty -> returns nil (caller skips
// the route, keeping the surface invisible instead of answering errors).
func (s *Server) mcpHandler() http.Handler {
	tok := mcpToken()
	if tok == "" {
		return nil
	}
	core := mcpserver.New(mcpserver.Config{
		Name:         "artex",
		Version:      BuildVersion,
		Instructions: mcpInstructions,
		Tools:        s.mcpTools(),
		Tiers:        []mcpserver.Tier{mcpserver.L1, mcpserver.L3},
	})
	return core.HTTPHandler(mcpserver.HTTPConfig{
		Authorize: func(r *http.Request) error {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if got == "" || got != tok {
				return errMCPUnauthorized
			}
			return nil
		},
		// Browser origins are never allowed: DNS-rebinding protection, and this
		// endpoint is for server-to-server harnesses, not web pages.
		AllowOrigin: func(origin string) bool { return false },
	})
}

var errMCPUnauthorized = jsonError("unauthorized")

type jsonError string

func (e jsonError) Error() string { return string(e) }

const mcpInstructions = `ARTEX is an autonomous penetration-testing console.
- task_create starts a new engagement task and returns its id; it runs asynchronously.
- task_status/task_list poll progress. A task finishes when its status is done/failed/timeout.
- Keep single tool calls under 60 seconds; these tools answer immediately.`

// mcpTools builds the fixed tool surface. task_create is the paperclip intake:
// description + goal in, task id out, launched on the normal creation path
// (CreateTaskWithOptions + launchTask — never raw SQL, per the ARTEX-5 lesson).
func (s *Server) mcpTools() []mcpserver.Tool {
	return []mcpserver.Tool{
		{
			Name:        "task_create",
			Title:       "Create a pentest task",
			Description: "Create an ARTEX task from a description and a goal. Starts immediately unless autostart=false (then it is created paused). Returns the task id for task_status polling.",
			Tier:        mcpserver.L3,
			ReadOnly:    false,
			Destructive: false,
			OpenWorld:   true,
			Schema: obj(map[string]any{
				"description": req("what the task is; becomes the task text (max 2000 chars)"),
				"goal":        req("the measurable goal the agents must prove"),
				"autostart":   opt("default true; false creates the task paused"),
			}),
			Handler: s.mcpTaskCreate,
		},
		{
			Name:        "task_status",
			Title:       "Get task status",
			Description: "Return one task's lifecycle status, paused flag, goal and description by task id.",
			Tier:        mcpserver.L3,
			ReadOnly:    true,
			Schema: obj(map[string]any{
				"task_id": req("task id returned by task_create"),
			}),
			Handler: s.mcpTaskStatus,
		},
		{
			Name:        "task_list",
			Title:       "List tasks",
			Description: "Return all non-archived tasks as id + name + status lines, most recent first.",
			Tier:        mcpserver.L1,
			ReadOnly:    true,
			Schema:      obj(map[string]any{"limit": opt("max lines, default 50")}),
			Handler:     s.mcpTaskList,
		},
	}
}

func req(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func opt(desc string) map[string]any { return map[string]any{"type": "boolean", "description": desc} }
func obj(props map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": reqList(props)}
}
func reqList(props map[string]any) (out []string) {
	for _, k := range []string{"description", "goal", "task_id"} {
		if _, ok := props[k]; ok {
			out = append(out, k)
		}
	}
	return out
}

func (s *Server) mcpTaskCreate(ctx context.Context, raw json.RawMessage) (string, error) {
	var in struct {
		Description string `json:"description"`
		Goal        string `json:"goal"`
		Autostart   *bool  `json:"autostart"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", err
	}
	in.Description = strings.TrimSpace(in.Description)
	in.Goal = strings.TrimSpace(in.Goal)
	if in.Description == "" || in.Goal == "" {
		return "", errMCPBadArgs
	}
	if len(in.Description) > 2000 {
		in.Description = in.Description[:2000]
	}
	t, err := s.m.CreateTaskWithOptions(in.Description, in.Goal, db.TaskCreateOptions{})
	if err != nil {
		return "", err
	}
	autostart := in.Autostart == nil || *in.Autostart
	if autostart {
		s.launchTask(t, in.Description+" "+in.Goal, false)
	} else if err := s.m.SetTaskPaused(t.ID, true); err != nil {
		return "", err
	}
	dto := taskDTO(t, s.resolvedTaskStatus(t))
	b, _ := json.Marshal(dto)
	return "task created: " + string(b), nil
}

var errMCPBadArgs = jsonError("description and goal are required")

func (s *Server) mcpTaskStatus(ctx context.Context, raw json.RawMessage) (string, error) {
	var in struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", err
	}
	t, ok := s.m.Task(strings.TrimSpace(in.TaskID))
	if !ok {
		return "", errMCPNoTask
	}
	b, _ := json.Marshal(taskDTO(t, s.resolvedTaskStatus(t)))
	return string(b), nil
}

var errMCPNoTask = jsonError("task not found")

func (s *Server) mcpTaskList(ctx context.Context, raw json.RawMessage) (string, error) {
	var in struct {
		Limit int `json:"limit"`
	}
	_ = json.Unmarshal(raw, &in)
	if in.Limit <= 0 || in.Limit > 200 {
		in.Limit = 50
	}
	rows, err := s.m.pg.ListTasks()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	n := 0
	for i := len(rows) - 1; i >= 0 && n < in.Limit; i-- {
		t := rows[i]
		st := "created"
		if live, ok := s.m.Task(strconv.FormatInt(t.ID, 10)); ok {
			st = s.resolvedTaskStatus(live)
		} else if t.Status != "" {
			st = t.Status
		}
		name := t.Name
		if name == "" {
			name = t.Description
		}
		if len(name) > 120 {
			name = name[:120] + "…"
		}
		b.WriteString("#" + strconv.FormatInt(t.ID, 10) + " [" + st + "] " + name + "\n")
		n++
	}
	return b.String(), nil
}
