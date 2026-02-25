package apischema

// EndpointParameter represents a single request parameter (path, query, body, etc.).
type EndpointParameter struct {
	Name        string                 `json:"name"`
	In          string                 `json:"in"` // path, query, header, body, etc.
	Description string                 `json:"description"`
	Required    bool                   `json:"required"`
	Type        string                 `json:"type"`
	Schema      map[string]interface{} `json:"schema"` // generic JSON schema
	Items       map[string]interface{} `json:"items"`  // for array types
}

// EndpointResponse describes a single HTTP response (e.g., 200, 400, 404).
type EndpointResponse struct {
	Description string                 `json:"description"`
	// Body holds an example response body parsed from the Postman example, when present.
	// It is kept generic so callers can either treat it as an example or derive a schema.
	Body map[string]interface{} `json:"body,omitempty"`
}

// Endpoint contains the full description of a single API operation.
// This structure is intentionally similar to a subset of Swagger/OpenAPI
// so that it is easy to reuse in other parts of the system.
type Endpoint struct {
	Method      string                       `json:"method"`
	Produces    []string                     `json:"produces"`
	Responses   map[string]EndpointResponse  `json:"responses"`
	Endpoint    string                       `json:"endpoint"` // URL path
	OperationID string                       `json:"operationId"`
	Consumes    []string                     `json:"consumes"`
	Parameters  []EndpointParameter          `json:"parameters"`
	Tags        []string                     `json:"tags"`
	Summary     string                       `json:"summary"`
	Description string                       `json:"description"`
}

// EndpointCollection holds all endpoints extracted from a Postman collection.
type EndpointCollection struct {
	Endpoints []Endpoint `json:"endpoints"`
}


