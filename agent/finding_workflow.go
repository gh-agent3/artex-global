package agent

import (
	"encoding/json"
	"fmt"

	"artex-global/db"
	actool "github.com/Autumn-27/norma/tool"
)

const findingIDGuidance = "\n\n**Finding id convention**: finding_id is the standalone finding record id; finding_node_id is the exploration node id. The id of list_findings / list_task_findings / node_detail / get_task_node_detail stays the exploration node id, and you read the standalone number from the finding_id of the same response. get_finding_traffic / bind_finding_traffic take the standalone finding_id. update_finding_report's finding_id argument still takes finding_node_id. Do not feed the number on the first line of report_finding to the evidence tools, and never guess another number after an id error."

// The server supplies the persisted setting. A missing setting/host is off.
// Consulted at assembly and again on writes so an already-running session
// cannot keep binding after the user switches the feature off.
var FindingTrafficBindingEnabled func() bool

func findingTrafficBindingEnabled() bool {
	return FindingTrafficBindingEnabled != nil && FindingTrafficBindingEnabled()
}

// Applied after ToolResolve: user descriptions and prompts remain intact, while
// all actual reporters (including Planner and custom chat agents) see the same
// API contract. Disabled/unbound tools are never reintroduced here.
func findingWorkflowTools(agentKey string, tools []actool.CoreTool) ([]actool.CoreTool, string) {
	if !findingTrafficBindingEnabled() {
		out := make([]actool.CoreTool, 0, len(tools))
		for _, tool := range tools {
			if tool.Name() == "bind_finding_traffic" {
				continue
			}
			if agentKey == "reporter" && (tool.Name() == "traffic_search" || tool.Name() == "traffic_get" || tool.Name() == "traffic_blob") {
				continue
			}
			switch tool.Name() {
			case "report_finding", "add_hint", "add_task_hint":
				// Work on a copy: toggling back on must restore the original schema.
				raw, _ := json.Marshal(tool.InputSchema())
				var schema map[string]any
				if json.Unmarshal(raw, &schema) == nil {
					stripTrafficParameters(schema)
					tool = DecorateTool(tool, tool.Description(), schema)
				}
			}
			out = append(out, tool)
		}
		return out, ""
	}
	out := append([]actool.CoreTool(nil), tools...)
	has := map[string]bool{}
	for i, tool := range out {
		has[tool.Name()] = true
		note := ""
		switch tool.Name() {
		case "report_finding":
			note = "\nBy default the report agent verifies and binds the traffic before writing the report. The reporter keeps the verification command, the key output, and any real traffic IDs with their purpose in evidence so the report agent can check them against the execution record; no extra packet hunting is needed for the binding. Explicit immediate binding is still supported: traffic_refs or evidence_hint_id may carry already-verified references (the latter reads the structured references of the named hint in this task); if any of them is invalid the whole report fails. TCP / no packets do not need these optional parameters. The returned finding_id and finding_node_id mean the standalone record and the exploration node respectively."
		case "add_hint", "add_task_hint":
			note = "\nWhen handing over a confirmed finding, keep the verified traffic IDs, their role, note and order in that hint's traffic_refs (top level for a single one, the matching hints element for a batch), and state in text which finding it proves. The caller must not hand over text only and drop existing traffic references. Unverified candidates must not be passed as evidence."
		case "get_finding_traffic", "bind_finding_traffic", "list_findings", "list_task_findings", "node_detail", "get_task_node_detail", "update_finding_report":
			note = findingIDGuidance
		}
		if note != "" {
			out[i] = DecorateTool(tool, tool.Description()+note, tool.InputSchema())
		}
	}
	guidance := ""
	if has["report_finding"] || has["add_task_hint"] || has["add_hint"] {
		guidance = "\n\n**Traffic evidence hand-off (optional)**: binding is done by default by the report agent after the finding is recorded and before the report is written. The reporter should keep the verification command, the key output, and any real traffic IDs with their purpose in evidence, and include the intent_id in the task so the report agent can trace it; do not go hunting for extra packets just to bind. When Auto / Planner reports on someone's behalf, do not drop the references the executor already has. add_hint / add_task_hint can hand over traffic_refs; explicit immediate binding through report_finding's traffic_refs / evidence_hint_id still works. Register normally for TCP or when nothing was captured - never guess an ID, and never re-probe only to fill a gap."
		if has["add_task_hint"] && !has["add_hint"] {
			guidance += "\nIn a platform conversation with no task context, do not call report_finding directly; hand it to the matching existing task with add_task_hint so that task's agent records it, then check the result with list_task_findings."
		}
		if has["prove_goal"] || has["goal_met"] {
			guidance += "\nBefore judging a goal complete, finish reporting/handing over the evidence you already have. Do not end the task or cancel Workers merely because the textual finding is recorded while the hand-off is unfinished; with no packets captured there is no need to wait or force a capture."
		}
	}
	if has["update_finding_report"] && has["bind_finding_traffic"] && has["get_finding_traffic"] {
		guidance += "\n\n**Traffic binding before the report (enabled)**: you are responsible for verifying and binding the traffic of the finding that triggered this run, then writing the report. First take the explicit finding_id and finding_node_id from the report_finding JSON or from get_task_node_detail / list_task_findings. Read the finding detail, the execution record of the intent that produced it and the existing evidence list, preferring the real IDs the reporter handed over. When this verification was HTTP and the traffic tools are available, filter candidates with traffic_search and then verify each request/response with traffic_get to confirm it really supports the finding; the domain and timestamp are only for filtering and do not prove ownership. Bind the confirmed evidence in reproduction order with bind_finding_traffic(finding_id, traffic_refs), choosing baseline / proof / verification / supporting and stating the purpose. Touch only this finding - do not create duplicate findings or re-probe the target. After a successful bind, call get_finding_traffic again for the latest version, read the bodies you need, and pass the version you actually read as evidence_version to update_finding_report (whose finding_id argument is still finding_node_id). Existing bindings need no extra entries. For TCP, when nothing was captured, when the tools are unavailable or when there is no exact match, skip automatic binding, write the report from the text/command evidence and state why - never guess to fill the gap. A failed bind is not a success; keep the existing evidence and say in the report why it was not bound."
	}
	if guidance != "" || has["get_finding_traffic"] || has["update_finding_report"] {
		guidance += findingIDGuidance
	}
	return out, guidance
}

func stripTrafficParameters(schema map[string]any) {
	props, _ := schema["properties"].(map[string]any)
	delete(props, "traffic_refs")
	delete(props, "evidence_hint_id")
	if required, ok := schema["required"].([]any); ok {
		kept := required[:0]
		for _, key := range required {
			if key != "traffic_refs" && key != "evidence_hint_id" {
				kept = append(kept, key)
			}
		}
		schema["required"] = kept
	}
	if hints, ok := props["hints"].(map[string]any); ok {
		if items, ok := hints["items"].(map[string]any); ok {
			stripTrafficParameters(items)
		}
	}
}

// HintTrafficSchema is shared by the task-local and cross-task hint tools.
func HintTrafficSchema() map[string]any {
	return map[string]any{"type": "array", "description": "optional: traffic references that are verified and match the specific finding in this hint, order preserved; after the hand-off report_finding can carry them via evidence_hint_id.", "items": obj(map[string]any{"traffic_id": str("real traffic id"), "role": str("baseline / proof / verification / supporting"), "note": str("what conclusion this traffic supports")}, "traffic_id")}
}

func (t *ToolSet) findingRefsFromHint(hintID int64, explicit []db.TrafficRef) ([]db.TrafficRef, error) {
	if hintID <= 0 {
		return db.NormalizeTrafficRefs(explicit)
	}
	n, err := t.ts.GetNode(hintID) // local store only: inherited hints cannot supply evidence
	if err != nil {
		return nil, err
	}
	if n == nil || n.Kind != db.KindHint {
		return nil, fmt.Errorf("evidence_hint_id=%d must be a hint node of this task (an inherited hint cannot be used for binding directly)", hintID)
	}
	var payload struct {
		Refs []db.TrafficRef `json:"traffic_refs"`
	}
	if err := json.Unmarshal(n.Payload, &payload); err != nil {
		return nil, err
	}
	return db.NormalizeTrafficRefs(append(append([]db.TrafficRef{}, explicit...), payload.Refs...))
}
