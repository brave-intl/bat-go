package responses

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// PaginationResponse - a response structure wrapper for pagination
type PaginationResponse struct {
	Page    int         `json:"page,omitempty"`
	Items   int         `json:"items,omitempty"`
	MaxPage int         `json:"max_page,omitempty"`
	Ordered []string    `json:"order,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// Render - render response
// response structure
// { page: 1, items: 50, max_page: 10, ordered: ["id", "..."], transactions: [...] }
func (pr *PaginationResponse) Render(ctx context.Context, w http.ResponseWriter, status int) error {
	// marshal response
	b, err := json.Marshal(pr)
	if err != nil {
		return fmt.Errorf("error encoding json response: %w", err)
	}

	// write response
	w.WriteHeader(status)
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("error writing response: %w", err)
	}
	return nil
}
