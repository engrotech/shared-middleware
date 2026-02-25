package apischema

// These structs model the parts of a Postman collection that we care about.
// They are intentionally minimal; if your collections contain more fields
// that are relevant, extend these structs accordingly.

type postmanCollection struct {
	Info struct {
		Name string `json:"name"`
	} `json:"info"`
	Item     []postmanItem     `json:"item"`
	Variable []postmanVariable `json:"variable,omitempty"` // Collection-level variables
}

// postmanVariable represents a collection-level variable in Postman.
type postmanVariable struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// postmanItem may be either a request item or a folder that contains items.
// We only care about leaf items that have a Request.
type postmanItem struct {
	Name     string        `json:"name"`
	Request  *postmanRequest `json:"request,omitempty"`
	Response []postmanResponse `json:"response,omitempty"`
	Item     []postmanItem `json:"item,omitempty"` // for folders
}

type postmanRequest struct {
	Method string           `json:"method"`
	URL    postmanURL       `json:"url"`
	Body   *postmanBody     `json:"body,omitempty"`
	Header []postmanKV      `json:"header,omitempty"`
}

type postmanURL struct {
	Raw   string            `json:"raw"`
	Path  []string          `json:"path,omitempty"`
	Query []postmanQueryParam `json:"query,omitempty"`
}

// postmanQueryParam represents a query parameter in a Postman URL.
type postmanQueryParam struct {
	Key         string `json:"key"`
	Value       string `json:"value,omitempty"`
	Description string `json:"description,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
}

type postmanBody struct {
	Mode      string                   `json:"mode"`
	Raw       string                   `json:"raw,omitempty"`       // for raw JSON / text bodies
	Formdata  []postmanFormDataItem    `json:"formdata,omitempty"` // for form-data mode
	Urlencoded []postmanFormDataItem    `json:"urlencoded,omitempty"` // for urlencoded mode
}

// postmanFormDataItem represents a form field in form-data or urlencoded body modes.
type postmanFormDataItem struct {
	Key         string `json:"key"`
	Value       string `json:"value,omitempty"`
	Type        string `json:"type,omitempty"`        // "text" or "file"
	Description string `json:"description,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
}

type postmanKV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// postmanResponse represents an example response saved in Postman.
// We primarily use the Body and optionally parse it as a schema/JSON example.
type postmanResponse struct {
	Name   string       `json:"name"`
	Status string       `json:"status"`
	Code   int          `json:"code"`
	Body   string       `json:"body"`
	Header []postmanKV  `json:"header,omitempty"`
}


