package server

import (
	"encoding/json"
	"io"

	"stingle-server/log"
)

// NewResponse returns a new StingleResponse with the given status.
func NewResponse(status string) *StingleResponse {
	return &StingleResponse{
		Status: status,
		Parts:  map[string]interface{}{},
		Infos:  []string{},
		Errors: []string{},
	}
}

// StingleResponse is the data structure used for most API calls.
// 'Status' is set to ok when the request was successful, and nok otherwise.
// 'Parts' contains any data returned to the caller.
// 'Infos' and 'Errors' are messages displayed to the user.
type StingleResponse struct {
	Status string                 `json:"status"`
	Parts  map[string]interface{} `json:"parts"`
	Infos  []string               `json:"infos"`
	Errors []string               `json:"errors"`
}

// AddPart adds a value to Parts.
func (r *StingleResponse) AddPart(name string, value interface{}) *StingleResponse {
	r.Parts[name] = value
	return r
}

// AddInfo adds a value to Infos.
func (r *StingleResponse) AddInfo(value string) *StingleResponse {
	r.Infos = append(r.Infos, value)
	return r
}

// AddError adds a value to Errors.
func (r *StingleResponse) AddError(value string) *StingleResponse {
	r.Errors = append(r.Errors, value)
	return r
}

// Send sends the StingleResponse.
func (r StingleResponse) Send(w io.Writer) error {
	if r.Status == "" {
		r.Status = "ok"
	}
	if r.Parts == nil {
		r.Parts = map[string]interface{}{}
	}
	if r.Infos == nil {
		r.Infos = []string{}
	}
	if r.Errors == nil {
		r.Errors = []string{}
	}
	j, err := json.Marshal(r)
	if err != nil {
		return err
	}
	log.Infof("StingleResponse: %s", j)
	_, err = w.Write(j)
	return err
}
