package server

import (
	"encoding/json"
	"log"
	"unicode"

	"github.com/gh-agent3/artex-global/agent"
	actool "github.com/Autumn-27/norma/tool"
)

// hasHan reports whether s still carries Chinese (Han) characters. It is the gate for
// the English-default migration below: a row whose text no longer contains Han has been
// rewritten by an operator and must be left alone.
func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// seedEnglishDefaults switches built-in prompts and built-in tool descriptions that still
// hold the original Chinese source over to the current English code defaults. The project
// source is Chinese-free, but the seeding path is first-insert only, so a database created
// earlier keeps the Chinese rows and the operator (and the model) would still see them.
//
// Two deliberate limits keep user work intact:
//   - only rows whose text still contains Han characters are touched;
//   - prompts go through ResetPromptToDefault, so the replaced version stays in the prompt
//     version log and can be restored from the UI.
//
// Settings-flag guarded, so it runs once. A fresh database needs nothing: SeedPromptIfEmpty
// and SeedTool already write the English defaults.
func (s *Server) seedEnglishDefaults() {
	const flag = "english_defaults_v4"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }()

	prompts := map[string]string{}
	for key, tmpl := range agent.BuiltinPromptSeeds() {
		prompts[key] = tmpl
	}
	// reporter and retester are seeded by their own one-off migrations, not by
	// BuiltinPromptSeeds, but their bodies are code-owned defaults too.
	prompts["reporter"] = agent.ReporterDefaultPrompt
	prompts["retester"] = agent.RetesterDefaultPrompt

	switchedPrompts := 0
	for key, tmpl := range prompts {
		a, err := s.m.pg.GetAgentByKey(key)
		if err != nil || a == nil || tmpl == "" {
			continue
		}
		cur, err := s.m.pg.CurrentPrompt(a.ID)
		if err != nil || cur == tmpl || !hasHan(cur) {
			continue
		}
		if _, err := s.m.pg.ResetPromptToDefault(a.ID, tmpl); err != nil {
			log.Printf("[prompts] failed to switch %s to the English default: %v", key, err)
			continue
		}
		switchedPrompts++
	}

	// Code-owned defaults for every built-in tool: the domain tools that seed the catalogue
	// plus the orchestration and platform host tools, which are seeded from the live registry.
	type toolDefault struct {
		desc   string
		schema map[string]any
	}
	defaults := make(map[string]toolDefault, len(agent.BuiltinToolSeeds()))
	for _, sd := range agent.BuiltinToolSeeds() {
		defaults[sd.Key] = toolDefault{desc: sd.Desc, schema: sd.Schema}
	}
	for _, t := range append(s.orchestrationTools(), s.platformTools()...) {
		defaults[t.Name()] = toolDefault{desc: t.Description(), schema: t.InputSchema()}
	}
	// The finding-workflow, finding-traffic, retest and captured-traffic tools are seeded
	// from their own server/traffic constructors, so pull their defaults in the same way.
	extra := append([]actool.CoreTool{s.toolBindFindingTraffic(), s.toolGetFindingTraffic()}, s.findingRetestTools()...)
	extra = append(extra, s.m.traffic.Tools()...)
	for _, t := range extra {
		defaults[t.Name()] = toolDefault{desc: t.Description(), schema: t.InputSchema()}
	}
	tools, err := s.m.pg.ListTools()
	if err != nil {
		log.Printf("[tools] English-default migration could not list tools: %v", err)
		return
	}
	switchedTools := 0
	for _, t := range tools {
		def, ok := defaults[t.Key]
		if !ok || def.desc == "" || !hasHan(t.Description) {
			continue
		}
		schema, err := json.Marshal(def.schema)
		if err != nil {
			continue
		}
		if err := s.m.pg.RefreshToolDefaults(t.Key, def.desc, schema); err != nil {
			log.Printf("[tools] failed to refresh %s to the English default: %v", t.Key, err)
			continue
		}
		switchedTools++
	}

	if switchedPrompts > 0 || switchedTools > 0 {
		log.Printf("[i18n] switched %d built-in prompt(s) and %d tool description(s) to the English defaults", switchedPrompts, switchedTools)
	}
}
