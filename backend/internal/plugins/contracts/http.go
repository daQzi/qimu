package contracts

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

type HTTPRequest struct {
	Method string             `json:"method"`
	Path   string             `json:"path"`
	Body   map[string]Binding `json:"body,omitempty"`
}
type HTTPAction struct {
	Submit     HTTPRequest       `json:"submit"`
	Poll       *HTTPRequest      `json:"poll,omitempty"`
	Lookup     *HTTPRequest      `json:"lookup,omitempty"`
	JobIDPath  string            `json:"jobIdPath,omitempty"`
	StatusPath string            `json:"statusPath,omitempty"`
	StatusMap  map[string]string `json:"statusMap,omitempty"`
	Outputs    map[string]string `json:"outputs"`
	Resources  map[string]string `json:"resources,omitempty"`
	Artifact   *struct {
		URLPath string `json:"urlPath"`
		Field   string `json:"field"`
		Kind    string `json:"kind"`
	} `json:"artifact,omitempty"`
	Idempotency struct {
		RetentionSeconds int    `json:"retentionSeconds,omitempty"`
		Mode             string `json:"mode"`
		Header           string `json:"header,omitempty"`
	} `json:"idempotency"`
	Cancellation struct {
		Mode    string       `json:"mode"`
		Request *HTTPRequest `json:"request,omitempty"`
	} `json:"cancellation"`
}
type HTTPConnector struct {
	ID        string `json:"id"`
	Transport string `json:"transport"`
	BaseURL   string `json:"baseUrl"`
	Auth      struct {
		Type   string `json:"type"`
		Header string `json:"header,omitempty"`
	} `json:"auth"`
	Actions map[string]HTTPAction `json:"actions"`
}

var httpName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,79}$`)
var httpHeader = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,79}$`)

// This validates inert author configuration. DNS, credential and ownership
// checks happen again against the selected connection at runtime.
func ValidateHTTPConnector(raw []byte) error {
	v, err := Decode(raw)
	if err != nil {
		return err
	}
	root, ok := v.(map[string]any)
	if !ok {
		return invalid("contract_invalid", "connector object")
	}
	if err = Validate("httpConnector", raw); err != nil {
		return err
	}
	var c HTTPConnector
	if err = json.Unmarshal(raw, &c); err != nil {
		return err
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return invalid("contract_invalid", "connector baseUrl")
	}
	if c.Auth.Type == "header" && (!httpHeader.MatchString(c.Auth.Header) || forbiddenHTTPHeader(c.Auth.Header)) {
		return invalid("contract_invalid", "auth header")
	}
	if c.Auth.Type != "header" && c.Auth.Header != "" {
		return invalid("contract_invalid", "unexpected auth header")
	}
	for _, a := range c.Actions {
		async := a.JobIDPath != ""
		if async != (a.Poll != nil) || async != (a.StatusPath != "") || async != (len(a.StatusMap) > 0) {
			return invalid("contract_invalid", "async job contract")
		}
		if !async && (a.Lookup != nil || a.Cancellation.Mode != "unsupported") {
			return invalid("contract_invalid", "sync recovery/cancellation")
		}
		for _, p := range append([]string{a.JobIDPath, a.StatusPath}, mapValues(a.Outputs)...) {
			if p != "" && !validHTTPPointer(p) {
				return invalid("contract_invalid", "response pointer")
			}
		}
		if a.Artifact != nil && (!validHTTPPointer(a.Artifact.URLPath) || !httpName.MatchString(a.Artifact.Field)) {
			return invalid("contract_invalid", "artifact mapping")
		}
		for key := range a.Outputs {
			if !httpName.MatchString(key) {
				return invalid("contract_invalid", "output field")
			}
		}
		for field := range a.Resources {
			if !httpName.MatchString(field) {
				return invalid("contract_invalid", "resource field")
			}
		}
		if a.Artifact != nil {
			if _, exists := a.Outputs[a.Artifact.Field]; exists {
				return invalid("contract_invalid", "duplicate artifact field")
			}
		}
		if a.Idempotency.Mode == "header" && (!httpHeader.MatchString(a.Idempotency.Header) || forbiddenHTTPHeader(a.Idempotency.Header) || strings.EqualFold(a.Idempotency.Header, c.Auth.Header)) {
			return invalid("contract_invalid", "idempotency header")
		}
		if a.Idempotency.Mode == "unsupported" && a.Idempotency.Header != "" {
			return invalid("contract_invalid", "unsupported idempotency header")
		}
		if a.Idempotency.Mode == "header" && (a.Idempotency.RetentionSeconds < 60 || a.Idempotency.RetentionSeconds > 86400) {
			return invalid("contract_invalid", "idempotency retentionSeconds required (60–86400)")
		}
		if (a.Cancellation.Mode == "request") != (a.Cancellation.Request != nil) {
			return invalid("contract_invalid", "cancel request")
		}
		for kind, request := range map[string]*HTTPRequest{"submit": &a.Submit, "poll": a.Poll, "lookup": a.Lookup, "cancel": a.Cancellation.Request} {
			if request == nil {
				continue
			}
			if err = validateHTTPRequest(*request, kind, a.Resources); err != nil {
				return err
			}
		}
	}
	_ = root
	return nil
}
func mapValues(m map[string]string) []string {
	values := make([]string, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}
func forbiddenHTTPHeader(value string) bool {
	switch strings.ToLower(value) {
	case "authorization", "cookie", "host", "content-type", "content-length", "connection", "transfer-encoding", "proxy-authorization", "proxy-connection":
		return true
	}
	return false
}
func validHTTPPointer(p string) bool {
	return strings.HasPrefix(p, "/") && len(p) <= 500 && !strings.ContainsAny(p, "\r\n")
}
func validateHTTPRequest(r HTTPRequest, kind string, resources map[string]string) error {
	if !strings.HasPrefix(r.Path, "/") || strings.HasPrefix(r.Path, "//") || strings.ContainsAny(r.Path, "?#\\\r\n") || strings.Contains(r.Path, "..") || strings.Contains(r.Path, "%") {
		return invalid("contract_invalid", "HTTP path")
	}
	path := r.Path
	if kind == "lookup" {
		path = strings.ReplaceAll(path, "{submissionKey}", "key")
		if !strings.Contains(r.Path, "{submissionKey}") {
			return invalid("contract_invalid", "lookup key missing")
		}
	}
	if kind == "poll" || kind == "cancel" {
		path = strings.ReplaceAll(path, "{jobId}", "job")
		if !strings.Contains(r.Path, "{jobId}") {
			return invalid("contract_invalid", "job id missing")
		}
	}
	if strings.ContainsAny(path, "{}") {
		return invalid("contract_invalid", "HTTP path variable")
	}
	if (kind == "submit" && r.Method != "POST") || ((kind == "poll" || kind == "lookup") && r.Method != "GET") || (kind == "cancel" && r.Method != "POST" && r.Method != "DELETE") {
		return invalid("contract_invalid", "HTTP method")
	}
	if kind != "submit" && len(r.Body) > 0 {
		return invalid("contract_invalid", "only submit maps body")
	}
	for key, binding := range r.Body {
		if !httpName.MatchString(key) {
			return invalid("contract_invalid", "body field")
		}
		if len(binding.Literal) > 0 {
			continue
		}
		if strings.HasPrefix(binding.From, "input.") && httpName.MatchString(strings.TrimPrefix(binding.From, "input.")) {
			continue
		}
		if strings.HasPrefix(binding.From, "resource.") {
			if _, ok := resources[strings.TrimPrefix(binding.From, "resource.")]; ok {
				continue
			}
		}
		return invalid("contract_invalid", fmt.Sprintf("unsupported mapping %s", key))
	}
	return nil
}

func ReadHTTPConnector(files PackageFiles, manifest Manifest, id string) (HTTPConnector, error) {
	for _, ref := range manifest.Contributes.Connectors {
		if ref.ID == id {
			var c HTTPConnector
			err := json.Unmarshal(files[ref.Ref], &c)
			return c, err
		}
	}
	return HTTPConnector{}, invalid("package_reference_invalid", "connector not found")
}
