package inputs

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/handlers"
)

// OrderDirection - the directionality type
type OrderDirection string

const (
	// Ascending - ASC
	Ascending OrderDirection = "ASC"
	// Descending - DESC
	Descending = "DESC"
)

// PageOrder - this directionality and attribute used for ordering
type PageOrder struct {
	Direction OrderDirection
	Attribute string
}

// Pagination - parameters common to pagination
// page=1&items=50&order=id
type Pagination struct {
	Order    []PageOrder
	RawOrder []string
	Page     int
	Items    int
}

// GetOrderBy - create the order by expression and parameters for pagination
func (p Pagination) GetOrderBy(count int) (string, []interface{}) {
	var (
		statement string
		v         = []interface{}{}
	)

	for _, po := range p.Order {
		if statement != "" {
			// not the first statement
			statement += ", "
		}
		statement += fmt.Sprintf("$%d ", count)
		v = append(v, po.Attribute)
		count++

		if po.Direction != "" {
			statement += fmt.Sprintf("$%d", count)
			v = append(v, po.Direction)
			count++
		}
	}
	return statement, v
}

// Validate - implementation of validatable interface
func (p *Pagination) Validate(ctx context.Context) error {
	var errs = new(errorutils.MultiError)
	if p.Page < 0 {
		errs.Append(errors.New("page value must be greater than 0"))
	}
	if p.Items < 0 {
		errs.Append(errors.New("items value must be greater than 0"))
	}

	// get allowed values for order from context, if nothing allow all values
	if okOrder, ok := ctx.Value(appctx.PaginationOrderOptionsCTXKey).(map[string]bool); ok {
		for _, o := range p.Order {
			if _, ok := okOrder[o.Attribute]; !ok {
				errs.Append(fmt.Errorf("order parameter '%s' is not allowed", o.Attribute))
			}
		}
	}

	if errs.Count() > 0 {
		return errs
	}

	return nil
}

// Decode - implementation of  decodable interface
func (p *Pagination) Decode(ctx context.Context, v []byte) error {
	u, err := url.Parse(string(v))
	if err != nil {
		return fmt.Errorf("failed to parse pagination parameters: %w", err)
	}

	// get the query string parameters
	q := u.Query()

	p.Page, err = strconv.Atoi(q.Get("page"))
	if err != nil {
		return fmt.Errorf("failed to parse pagination page parameter: %w", err)
	}

	p.Items, err = strconv.Atoi(q.Get("items"))
	if err != nil {
		return fmt.Errorf("failed to parse pagination items parameter: %w", err)
	}

	for _, v := range q["order"] {
		parts := strings.Split(v, ".")
		po := PageOrder{}
		if parts[0] != "" {
			po.Attribute = parts[0]
		}
		if len(parts) > 1 && parts[1] != "" {
			if string(Ascending) == strings.ToUpper(parts[1]) {
				po.Direction = Ascending
			}
			if string(Descending) == strings.ToUpper(parts[1]) {
				po.Direction = Descending
			}
		}
		p.Order = append(p.Order, po)
	}
	p.RawOrder = q["order"]

	return nil
}

// NewPagination - create a new Pagination struct and populate from url and order options
func NewPagination(ctx context.Context, url string, orderOptions ...string) (*Pagination, error) {
	var (
		pagination = new(Pagination)
		order      = map[string]bool{}
	)
	// allowed pagination ordering attributes
	for _, o := range orderOptions {
		order[o] = true
	}
	ctx = context.WithValue(ctx, appctx.PaginationOrderOptionsCTXKey, order)

	if err := DecodeAndValidate(ctx, pagination, []byte(url)); err != nil {
		fmt.Println("!!!!! err: ", err)
		return nil, handlers.ValidationError(
			"Error decoding or validating request pagination url parameter",
			map[string]interface{}{
				"pagination": "pagination failed validation",
			},
		)
	}
	return pagination, nil
}
