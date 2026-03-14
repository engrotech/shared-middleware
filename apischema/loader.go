package apischema

import (
	"encoding/json"
	"log"
	"regexp"
	"strconv"
	"strings"
)

// Loader is responsible for turning a Postman collection JSON into an
// EndpointCollection structure.
type Loader struct {
	collection        EndpointCollection
	variables         map[string]string // Collection-level variables for resolution
	postmanGlobalVars map[string]string // Postman global variables from frontend (postmanGlobalVariables)
}

// NewLoader creates a new Loader with an empty collection.
func NewLoader() *Loader {
	return &Loader{
		collection: EndpointCollection{
			Endpoints: make([]Endpoint, 0),
		},
		variables:         make(map[string]string),
		postmanGlobalVars: make(map[string]string),
	}
}

// SetPostmanGlobalVariables sets the postmanGlobalVariables that should be used
// to replace {{variableName}} placeholders in endpoints.
func (l *Loader) SetPostmanGlobalVariables(vars map[string]string) {
	if vars == nil {
		l.postmanGlobalVars = make(map[string]string)
	} else {
		l.postmanGlobalVars = vars
	}
}

// Collection returns the in-memory EndpointCollection.
func (l *Loader) Collection() EndpointCollection {
	log.Printf("apischema: Returning collection with %d endpoints", len(l.collection.Endpoints))
	return l.collection
}

// LoadFromBytes parses a Postman collection JSON (as bytes), extracts
// request/response schemas, and stores them in the loader's collection.
// It also extracts collection-level variables for resolution.
func (l *Loader) LoadFromBytes(data []byte) error {
	var coll postmanCollection
	if err := json.Unmarshal(data, &coll); err != nil {
		return err
	}

	// Extract collection-level variables
	l.variables = make(map[string]string)
	for _, v := range coll.Variable {
		if v.Key != "" {
			l.variables[v.Key] = v.Value
		}
	}

	endpoints := make([]Endpoint, 0)
	for _, item := range coll.Item {
		l.collectEndpointsFromItem(&endpoints, item)
	}

	l.collection.Endpoints = endpoints

	// Log the final structure for observability.
	// l.logCollection()

	return nil
}

// collectEndpointsFromItem traverses the Postman item tree and appends any
// leaf request items as endpoints.
func (l *Loader) collectEndpointsFromItem(out *[]Endpoint, item postmanItem) {
	// Folder case: recurse into children.
	if item.Request == nil && len(item.Item) > 0 {
		for _, child := range item.Item {
			l.collectEndpointsFromItem(out, child)
		}
		return
	}

	// Leaf request item.
	if item.Request == nil {
		return
	}

	endpoint := l.buildEndpointFromRequest(item)
	*out = append(*out, endpoint)
}

// buildEndpointFromRequest converts a single Postman item into our Endpoint model.
// It ensures all variables are resolved in the endpoint path before returning.
func (l *Loader) buildEndpointFromRequest(item postmanItem) Endpoint {
	req := item.Request

	method := strings.ToLower(req.Method)
	path := l.extractPath(req.URL)

	parameters := l.buildParameters(req)
	responses := l.buildResponses(item.Response)

	consumes := l.inferConsumes(req)
	produces := l.inferProduces(item.Response)

	return Endpoint{
		Method:      method,
		Produces:    produces,
		Responses:   responses,
		Endpoint:    path,
		OperationID: l.buildOperationID(item, method, path),
		Consumes:    consumes,
		Parameters:  parameters,
		Tags:        []string{}, // can be extended from item name or folders
		Summary:     item.Name,
		Description: "", // Postman items may hold description; extend if needed
	}
}

// resolveVariables replaces {{variableName}} placeholders with actual values from collection variables.
func (l *Loader) resolveVariables(value string) string {
	if value == "" || len(l.variables) == 0 {
		return value
	}

	// Match {{variableName}} pattern
	varRegex := regexp.MustCompile(`\{\{([^}]+)\}\}`)
	result := varRegex.ReplaceAllStringFunc(value, func(match string) string {
		// Extract variable name (remove {{ and }})
		varName := strings.TrimPrefix(strings.TrimSuffix(match, "}}"), "{{")
		varName = strings.TrimSpace(varName)

		// Look up variable value
		if resolvedValue, exists := l.variables[varName]; exists {
			return resolvedValue
		}
		// If variable not found, return original placeholder
		return match
	})

	return result
}

// resolveVariablesInMap recursively resolves variables in nested JSON structures (maps, arrays, strings).
func (l *Loader) resolveVariablesInMap(data interface{}) interface{} {
	if len(l.variables) == 0 {
		return data
	}

	switch v := data.(type) {
	case string:
		return l.resolveVariables(v)
	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, value := range v {
			result[key] = l.resolveVariablesInMap(value)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = l.resolveVariablesInMap(item)
		}
		return result
	default:
		return data
	}
}

// extractPath chooses a path representation from Postman URL.
func (l *Loader) extractPath(u postmanURL) string {
	var path string

	if u.Raw != "" {
		// Resolve variables in raw URL first
		resolvedRaw := l.resolveVariables(u.Raw)

// Raw often contains full URL; we want just the path portion if possible.
		// if strings.HasPrefix(resolvedRaw, "http://") || strings.HasPrefix(resolvedRaw, "https://") {
		// 	// Try to parse via net/http's Request.
		// 	// req, err := http.NewRequest(http.MethodGet, resolvedRaw, nil)
		// 	// if err == nil {
		// 	// 	path = req.URL.Path
		// 	// } else {
		// 	// 	path = resolvedRaw
		// 	// }
		// 	skipLen := 8 // https://
		// 	if strings.HasPrefix(resolvedRaw, "http://") {
		// 		skipLen = 7 // http://
		// 	}
		// 	resolvedPath := resolvedRaw[skipLen:]
		// 	path = resolvedPath

		// } else {
		// 	path = resolvedRaw
		// }		
		// Preserve full URL with protocol (don't remove http:// or https://)
		path = resolvedRaw
	} else if len(u.Path) > 0 {
		// Resolve variables in each path segment
		resolvedPath := make([]string, 0, len(u.Path))
		for _, segment := range u.Path {
			resolvedPath = append(resolvedPath, l.resolveVariables(segment))
		}
		path = "/" + strings.Join(resolvedPath, "/")
	}

	// Resolve any remaining variables in the final path
	return l.resolveVariables(path)
}

// buildParameters collects parameters from the request body, URL, and headers.
// It extracts:
//   - path parameters from URL path segments with `{param}` syntax
//   - query parameters from the URL query string
//   - header parameters from request headers (including all headers: Cookie, origin, sec-*, etc.)
//   - body parameters from raw JSON request body
func (l *Loader) buildParameters(req *postmanRequest) []EndpointParameter {
	params := make([]EndpointParameter, 0)

	// Path parameters from the URL path containing `{param}` segments.
	path := l.extractPath(req.URL)
	for _, segment := range strings.Split(path, "/") {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			name := strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")
			params = append(params, EndpointParameter{
				Name:        name,
				In:          "path",
				Description: "",
				Required:    true,
				Type:        "string",
				Schema:      nil,
				Items:       nil,
			})
		}
	}

	// Query parameters from the URL query string.
	if req.URL.Query != nil {
		for _, q := range req.URL.Query {
			// Skip disabled query parameters
			if q.Disabled {
				continue
			}
			// Resolve variables in query parameter value
			resolvedValue := l.resolveVariables(q.Value)
			params = append(params, EndpointParameter{
				Name:        q.Key,
				In:          "query",
				Description: q.Description,
				Required:    false, // Query parameters are typically optional unless specified
				Type:        "string",
				Schema:      nil,
				Items:       nil,
			})
			// Note: resolvedValue is available if needed for schema extraction
			_ = resolvedValue
		}
	}

	// Header parameters from request headers (including all headers).
	if req.Header != nil {
		for _, h := range req.Header {
			// Extract ALL headers without filtering (including Cookie, origin, sec-* headers, etc.)
			// Resolve variables in header value
			resolvedValue := l.resolveVariables(h.Value)
			params = append(params, EndpointParameter{
				Name:        h.Key,
				In:          "header",
				Description: "",
				Required:    false, // Headers are typically optional unless specified
				Type:        "string",
				Schema:      nil,
				Items:       nil,
			})
			// Note: resolvedValue is not stored in the parameter, but variables are resolved
			// for schema extraction purposes. The original variable name is preserved in the parameter name.
			_ = resolvedValue // Use resolved value if needed in future
		}
	}

	// Body parameter: handle different body modes (raw, formdata, urlencoded)
	if req.Body != nil {
		switch req.Body.Mode {
		case "raw":
			// Raw JSON/text body: try to decode as JSON schema
			if strings.TrimSpace(req.Body.Raw) != "" {
				// Resolve variables in raw body before parsing
				resolvedRaw := l.resolveVariables(req.Body.Raw)

				schema := make(map[string]interface{})
				if err := json.Unmarshal([]byte(resolvedRaw), &schema); err != nil {
					// If it's not valid JSON, just keep it as a string under a "example" field.
					schema["example"] = resolvedRaw
				}

				params = append(params, EndpointParameter{
					Name:        "body",
					In:          "body",
					Description: "",
					Required:    true,
					Type:        "",
					Schema:      schema,
					Items:       nil,
				})
			}
		case "formdata":
			// Form-data body: extract each field as a separate parameter
			if req.Body.Formdata != nil {
				for _, field := range req.Body.Formdata {
					if field.Disabled {
						continue
					}
					// Resolve variables in field value
					resolvedValue := l.resolveVariables(field.Value)

					// Create a schema map for this field
					fieldSchema := make(map[string]interface{})
					if resolvedValue != "" {
						fieldSchema["example"] = resolvedValue
					}
					if field.Type != "" {
						fieldSchema["type"] = field.Type
					}

					params = append(params, EndpointParameter{
						Name:        field.Key,
						In:          "body",
						Description: field.Description,
						Required:    false, // Form fields are typically optional unless specified
						Type:        field.Type,
						Schema:      fieldSchema,
						Items:       nil,
					})
				}
			}
		case "urlencoded":
			// URL-encoded body: extract each field as a separate parameter
			if req.Body.Urlencoded != nil {
				for _, field := range req.Body.Urlencoded {
					if field.Disabled {
						continue
					}
					// Resolve variables in field value
					resolvedValue := l.resolveVariables(field.Value)

					// Create a schema map for this field
					fieldSchema := make(map[string]interface{})
					if resolvedValue != "" {
						fieldSchema["example"] = resolvedValue
					}
					if field.Type != "" {
						fieldSchema["type"] = field.Type
					}

					params = append(params, EndpointParameter{
						Name:        field.Key,
						In:          "body",
						Description: field.Description,
						Required:    false, // URL-encoded fields are typically optional unless specified
						Type:        field.Type,
						Schema:      fieldSchema,
						Items:       nil,
					})
				}
			}
		}
	}

	return params
}

// buildResponses converts Postman saved responses into our response map.
// Since Postman examples do not directly provide descriptions, we use the
// status text if available or a generic description.
func (l *Loader) buildResponses(responses []postmanResponse) map[string]EndpointResponse {
	if len(responses) == 0 {
		return map[string]EndpointResponse{}
	}

	out := make(map[string]EndpointResponse)
	for _, r := range responses {
		codeStr := ""
		if r.Code > 0 {
			codeStr = strings.TrimSpace(strconv.Itoa(r.Code))
		}
		if codeStr == "" {
			continue
		}

		desc := r.Status
		if desc == "" {
			desc = "Response " + codeStr
		}

		var bodyExample map[string]interface{}
		rawBody := strings.TrimSpace(r.Body)
		if rawBody != "" {
			// Try to parse the Postman example body as JSON.
			var parsed interface{}
			if err := json.Unmarshal([]byte(rawBody), &parsed); err == nil {
				if m, ok := parsed.(map[string]interface{}); ok {
					bodyExample = m
				} else {
					// If it's valid JSON but not an object, wrap it.
					bodyExample = map[string]interface{}{"_value": parsed}
				}
			} else {
				// Not valid JSON; store as raw string so callers still see something.
				bodyExample = map[string]interface{}{"_raw": rawBody}
			}
		}

		out[codeStr] = EndpointResponse{
			Description: desc,
			Body:        bodyExample,
		}
	}

	return out
}

// inferConsumes guesses request content types from headers or body mode.
func (l *Loader) inferConsumes(req *postmanRequest) []string {
	types := make([]string, 0)

	for _, h := range req.Header {
		if strings.EqualFold(h.Key, "Content-Type") {
			// Resolve variables in Content-Type header value
			resolvedValue := l.resolveVariables(h.Value)
			types = append(types, resolvedValue)
		}
	}

	// Fallbacks
	if len(types) == 0 && req.Body != nil && req.Body.Mode == "raw" {
		// Assume JSON if body looks like JSON.
		// Resolve variables first
		resolvedRaw := l.resolveVariables(req.Body.Raw)
		raw := strings.TrimSpace(resolvedRaw)
		if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
			types = append(types, "application/json")
		}
	}

	return types
}

// inferProduces guesses response content types from example response headers.
func (l *Loader) inferProduces(responses []postmanResponse) []string {
	set := make(map[string]struct{})

	for _, r := range responses {
		for _, h := range r.Header {
			if strings.EqualFold(h.Key, "Content-Type") {
				set[h.Value] = struct{}{}
			}
		}
	}

	if len(set) == 0 && len(responses) > 0 {
		// Assume JSON by default if there are responses but no explicit Content-Type.
		set["application/json"] = struct{}{}
	}

	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	return out
}

// buildOperationID creates a stable operationId from item name/method/path.
func (l *Loader) buildOperationID(item postmanItem, method, path string) string {
	if item.Name != "" {
		// Prefer item name with spaces removed.
		return sanitizeOperationID(item.Name)
	}
	if method != "" && path != "" {
		return sanitizeOperationID(method + "_" + path)
	}
	return ""
}

// sanitizeOperationID makes a string safe for use as an operationId.
func sanitizeOperationID(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "{", "")
	s = strings.ReplaceAll(s, "}", "")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

// isInternalHeader checks if a header is an internal/standard HTTP header
// that should not be treated as a custom parameter.
func isInternalHeader(headerName string) bool {
	internalHeaders := map[string]bool{
		"content-type":    true,
		"content-length":  true,
		"accept":          true,
		"accept-encoding": true,
		"accept-language": true,
		// "authorization":     true,
		"cache-control":       true,
		"connection":          true,
		"cookie":              true,
		"date":                true,
		"expect":              true,
		"host":                true,
		"if-match":            true,
		"if-modified-since":   true,
		"if-none-match":       true,
		"if-range":            true,
		"if-unmodified-since": true,
		"max-forwards":        true,
		"pragma":              true,
		"proxy-authorization": true,
		"range":               true,
		"referer":             true,
		"referrer":            true,
		"te":                  true,
		"trailer":             true,
		"transfer-encoding":   true,
		"upgrade":             true,
		"user-agent":          true,
		"via":                 true,
		"warning":             true,
		"x-forwarded-for":     true,
		"x-forwarded-proto":   true,
		"x-real-ip":           true,
	}
	return internalHeaders[strings.ToLower(headerName)]
}

// logCollection logs the current endpoint collection as JSON.
func (l *Loader) logCollection() {
	_, err := json.MarshalIndent(l.collection, "", "  ")
	if err != nil {
		log.Printf("apischema: failed to marshal endpoint collection: %v", err)
		return
	}
	// log.Printf("apischema: extracted endpoints: %s", string(b))
}
