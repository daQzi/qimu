package mediaanalysis

import (
	"encoding/json"
	"strings"
	"testing"
)

func validReport() Report {
	return Report{SchemaVersion: 1, Source: Source{"owned-video", strings.Repeat("a", 64), 60000}, Coverage: Span{0, 10000}, AudioAnalyzed: true,
		Shots:       []Shot{{ID: "shot-1", Span: Span{0, 10000}, Description: "人物进入房间", Camera: "固定镜头", Uncertainties: []string{}}},
		Entities:    []Entity{{ID: "person-1", Kind: "character", Name: "人物一", Description: "画面中的人物", Evidence: []Evidence{{"shot-1", 1000, "人物出现在画面中央"}}, Uncertainties: []string{}}},
		Dialogue:    []Utterance{{ID: "line-1", Span: Span{1000, 3000}, Text: "测试台词", SpeakerID: "person-1", EvidenceKind: "audio", Uncertainties: []string{}}},
		Limitations: []string{}}
}
func TestReportReferencesAndCoverage(t *testing.T) {
	report := validReport()
	raw, _ := json.Marshal(report)
	if _, err := Decode(raw, report.Source); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*Report)
	}{
		{"wrong source", func(r *Report) { r.Source.ResourceID = "foreign" }},
		{"schema mismatch", func(r *Report) { r.SchemaVersion = 2 }},
		{"coverage overflow", func(r *Report) { r.Coverage.EndMs = 60001 }},
		{"shot overflow", func(r *Report) { r.Shots[0].EndMs = 10001 }},
		{"duplicate shot", func(r *Report) { r.Shots = append(r.Shots, r.Shots[0]) }},
		{"missing evidence", func(r *Report) { r.Entities[0].Evidence[0].ShotID = "missing" }},
		{"evidence overflow", func(r *Report) { r.Entities[0].Evidence[0].AtMs = 10000 }},
		{"duplicate entity", func(r *Report) { r.Entities = append(r.Entities, r.Entities[0]) }},
		{"speaker missing", func(r *Report) { r.Dialogue[0].SpeakerID = "missing" }},
		{"speaker is prop", func(r *Report) { r.Entities[0].Kind = "prop" }},
		{"audio fabricated", func(r *Report) { r.AudioAnalyzed = false; r.Limitations = []string{"只分析画面"} }},
		{"duplicate dialogue", func(r *Report) { r.Dialogue = append(r.Dialogue, r.Dialogue[0]) }},
		{"null facts", func(r *Report) { r.Entities = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := validReport()
			source := r.Source
			tc.change(&r)
			if err := Validate(r, source); err == nil {
				t.Fatal("invalid evidence accepted")
			}
		})
	}
}
func TestReportNoImplicitRepair(t *testing.T) {
	report := validReport()
	raw, _ := json.Marshal(report)
	for _, bad := range []string{"分析如下：" + string(raw), string(raw) + " {}", string(raw[:len(raw)-1]) + `,"unknown":true}`, string(raw[:len(raw)-1]) + `,"schemaVersion":1}`, strings.Replace(string(raw), `"startMs":0`, `"startMs":12,"startMs":0`, 1), "null", strings.Repeat("x", MaxReportBytes+1)} {
		if _, err := Decode([]byte(bad), report.Source); err == nil {
			t.Fatal("malformed output accepted")
		}
	}
	report.AudioAnalyzed = false
	report.Dialogue[0].EvidenceKind = "subtitle"
	report.Dialogue[0].SpeakerID = ""
	report.Limitations = []string{"未分析音轨，文字仅来自字幕"}
	if err := Validate(report, report.Source); err != nil {
		t.Fatal("explicit uncertainty rejected", err)
	}
}
