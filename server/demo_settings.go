package server

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

// Demo mode settings are global (shared by everyone using this instance), unlike the
// per-browser on/off state: the classes decide WHAT counts as sensitive and the rules
// decide which extra words must be masked, so both belong to the instance.
const (
	settingDemoClasses = "demo_classes"
	settingDemoRules   = "demo_rules"
)

// demoDefaultClasses mirrors the plan: target hosts are masked by default too, because a
// client demo usually must not reveal which site was tested.
var demoDefaultClasses = []string{"secret", "identity", "target", "evidence"}

type demoSettingsPayload struct {
	Classes []string `json:"classes"`
	Rules   []string `json:"rules"`
}

func (s *Server) readDemoSettings() demoSettingsPayload {
	out := demoSettingsPayload{Classes: demoDefaultClasses, Rules: []string{}}
	if raw, _, _ := s.m.pg.GetSetting(settingDemoClasses); strings.TrimSpace(raw) != "" {
		var classes []string
		if json.Unmarshal([]byte(raw), &classes) == nil && len(classes) > 0 {
			out.Classes = classes
		}
	}
	if raw, _, _ := s.m.pg.GetSetting(settingDemoRules); strings.TrimSpace(raw) != "" {
		var rules []string
		if json.Unmarshal([]byte(raw), &rules) == nil {
			out.Rules = rules
		}
	}
	return out
}

// demoSettings returns the masking classes and the custom masking rules.
func (s *Server) demoSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.readDemoSettings())
}

// demoSettingsSave stores the classes and rules. Rules are validated as regular
// expressions here, so a broken pattern is rejected at save time instead of silently
// failing to mask during a demo.
func (s *Server) demoSettingsSave(w http.ResponseWriter, r *http.Request) {
	var req demoSettingsPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	allowed := map[string]bool{"secret": true, "identity": true, "target": true, "evidence": true}
	classes := make([]string, 0, len(req.Classes))
	for _, c := range req.Classes {
		c = strings.TrimSpace(c)
		if allowed[c] {
			classes = append(classes, c)
		}
	}
	rules := make([]string, 0, len(req.Rules))
	for _, rule := range req.Rules {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		if _, err := regexp.Compile(rule); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid rule "+rule+": "+err.Error())
			return
		}
		rules = append(rules, rule)
	}
	classesJSON, _ := json.Marshal(classes)
	rulesJSON, _ := json.Marshal(rules)
	if err := s.m.pg.SetSetting(settingDemoClasses, string(classesJSON)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.m.pg.SetSetting(settingDemoRules, string(rulesJSON)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, demoSettingsPayload{Classes: classes, Rules: rules})
}
