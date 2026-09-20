// Package mediaanalysis validates evidence-backed video reports independently
// of a model vendor, Agent prompt, or plugin package.
package mediaanalysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

const MaxReportBytes = 64 << 10

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,79}$`)
var digest = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Source comes from the host resource/probe, never a model's estimate.
type Source struct {
	ResourceID string `json:"resourceId"`
	Digest     string `json:"digest"`
	DurationMs int64  `json:"durationMs"`
}
type Span struct {
	StartMs int64 `json:"startMs"`
	EndMs   int64 `json:"endMs"`
}
type Shot struct {
	ID string `json:"id"`
	Span
	Description   string   `json:"description"`
	Camera        string   `json:"camera"`
	Uncertainties []string `json:"uncertainties"`
}
type Evidence struct {
	ShotID      string `json:"shotId"`
	AtMs        int64  `json:"atMs"`
	Description string `json:"description"`
}
type Entity struct {
	ID            string     `json:"id"`
	Kind          string     `json:"kind"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Evidence      []Evidence `json:"evidence"`
	Uncertainties []string   `json:"uncertainties"`
}
type Utterance struct {
	ID string `json:"id"`
	Span
	Text          string   `json:"text"`
	SpeakerID     string   `json:"speakerId"`
	EvidenceKind  string   `json:"evidenceKind"`
	Uncertainties []string `json:"uncertainties"`
}
type Report struct {
	SchemaVersion int         `json:"schemaVersion"`
	Source        Source      `json:"source"`
	Coverage      Span        `json:"coverage"`
	AudioAnalyzed bool        `json:"audioAnalyzed"`
	Shots         []Shot      `json:"shots"`
	Entities      []Entity    `json:"entities"`
	Dialogue      []Utterance `json:"dialogue"`
	Limitations   []string    `json:"limitations"`
}

func text(v string, max int) bool {
	return utf8.ValidString(v) && v != "" && strings.TrimSpace(v) == v && utf8.RuneCountInString(v) <= max
}
func notes(values []string) bool {
	if values == nil || len(values) > 20 {
		return false
	}
	for _, v := range values {
		if !text(v, 500) {
			return false
		}
	}
	return true
}
func within(span, outer Span) bool {
	return span.StartMs >= outer.StartMs && span.EndMs > span.StartMs && span.EndMs <= outer.EndMs
}

// Decode accepts exactly one JSON document. It never repairs prose or retries
// the paid model call on the caller's behalf.
func Decode(raw []byte, source Source) (Report, error) {
	var report Report
	if len(raw) == 0 || len(raw) > MaxReportBytes {
		return report, fmt.Errorf("分析结果须为不超过 64 KiB 的 JSON")
	}
	if !utf8.Valid(raw) {
		return report, fmt.Errorf("分析结果必须为 UTF-8")
	}
	if err := uniqueFields(json.NewDecoder(bytes.NewReader(raw)), 0); err != nil {
		return report, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return report, fmt.Errorf("分析结果 JSON 不符合合同: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return report, fmt.Errorf("分析结果必须只有一个 JSON 对象")
	}
	if err := Validate(report, source); err != nil {
		return report, err
	}
	return report, nil
}

// Inspect object keys before struct decoding, which otherwise accepts duplicate
// fields using last-value-wins semantics. Bound depth before recursing.
func uniqueFields(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return fmt.Errorf("分析结果嵌套过深")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("分析结果包含重复字段")
			}
			seen[name] = true
			if err = uniqueFields(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err = uniqueFields(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("分析结果 JSON 结构无效")
	}
	_, err = decoder.Token()
	return err
}

func Validate(report Report, source Source) error {
	if !text(source.ResourceID, 80) || !digest.MatchString(source.Digest) || source.DurationMs <= 0 {
		return fmt.Errorf("缺少宿主核验的原片信息")
	}
	if report.SchemaVersion != 1 || report.Source != source {
		return fmt.Errorf("分析结果类型、版本或原片来源不匹配")
	}
	if !within(report.Coverage, Span{EndMs: source.DurationMs}) {
		return fmt.Errorf("分析覆盖范围超出原片")
	}
	if len(report.Shots) == 0 || len(report.Shots) > 200 || report.Entities == nil || len(report.Entities) > 100 || report.Dialogue == nil || len(report.Dialogue) > 300 || !notes(report.Limitations) {
		return fmt.Errorf("镜头、实体、对白或限制列表超出合同")
	}
	shots := map[string]Shot{}
	entities := map[string]Entity{}
	utterances := map[string]bool{}
	var previousEnd int64 = -1
	for _, shot := range report.Shots {
		if !identifier.MatchString(shot.ID) || !within(shot.Span, report.Coverage) || shot.StartMs < previousEnd || !text(shot.Description, 1000) || !text(shot.Camera, 500) || !notes(shot.Uncertainties) {
			return fmt.Errorf("镜头 %q 的编号、时间顺序或描述无效", shot.ID)
		}
		if _, exists := shots[shot.ID]; exists {
			return fmt.Errorf("镜头编号重复: %s", shot.ID)
		}
		shots[shot.ID] = shot
		previousEnd = shot.EndMs
	}
	for _, entity := range report.Entities {
		if !identifier.MatchString(entity.ID) || (entity.Kind != "character" && entity.Kind != "scene" && entity.Kind != "prop") || !text(entity.Name, 160) || !text(entity.Description, 1000) || len(entity.Evidence) == 0 || len(entity.Evidence) > 100 || !notes(entity.Uncertainties) {
			return fmt.Errorf("实体 %q 的类型、字段或证据无效", entity.ID)
		}
		if _, exists := entities[entity.ID]; exists {
			return fmt.Errorf("实体编号重复: %s", entity.ID)
		}
		for _, e := range entity.Evidence {
			shot, exists := shots[e.ShotID]
			if !exists || e.AtMs < shot.StartMs || e.AtMs >= shot.EndMs || !text(e.Description, 500) {
				return fmt.Errorf("实体 %s 的镜头证据不存在或越界", entity.ID)
			}
		}
		entities[entity.ID] = entity
	}
	for _, line := range report.Dialogue {
		if !identifier.MatchString(line.ID) || utterances[line.ID] || !within(line.Span, report.Coverage) || !text(line.Text, 2000) || !notes(line.Uncertainties) {
			return fmt.Errorf("对白 %q 的编号、时间或内容无效", line.ID)
		}
		if line.EvidenceKind != "audio" && line.EvidenceKind != "subtitle" {
			return fmt.Errorf("对白来源只能为音轨或画面字幕")
		}
		if line.EvidenceKind == "audio" && !report.AudioAnalyzed {
			return fmt.Errorf("未分析音轨，不能声明音轨对白")
		}
		if line.SpeakerID != "" {
			entity, exists := entities[line.SpeakerID]
			if !exists || entity.Kind != "character" {
				return fmt.Errorf("对白说话人必须引用已识别人物或留空待确认")
			}
		}
		utterances[line.ID] = true
	}
	if !report.AudioAnalyzed && len(report.Limitations) == 0 {
		return fmt.Errorf("未分析音轨时必须说明分析限制")
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return err
	}
	if len(raw) > MaxReportBytes {
		return fmt.Errorf("分析结果超过 64 KiB")
	}
	return nil
}
