package inputs

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	appctx "github.com/brave-intl/bat-go/libs/context"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/handlers"
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
func (p Pagination) GetOrderBy(ctx context.Context) string {
	var (
		statement string
	)
	// get allowed values for order from context, if nothing allow all values
	if okOrder, ok := ctx.Value(appctx.PaginationOrderOptionsCTXKey).(map[string]string); ok {
		for _, po := range p.Order {
			if statement != "" {
				// not the first statement
				statement += ", "
			}
			statement += fmt.Sprintf("%s ", okOrder[po.Attribute])

			if po.Direction != "" {
				if po.Direction == Ascending {
					statement += " ASC "
				} else if po.Direction == Descending {
					statement += " DESC "
				}
			}
		}
	}
	return statement
}

// Validate - implementation of validatable interface
func (p *Pagination) Validate(ctx context.Context) error {
	var errs = new(errorutils.MultiError)
	if p.Page < 0 {
		errs.Append(errors.New("page value must be greater than or equal to 0"))
	}
	if p.Items <= 0 {
		errs.Append(errors.New("items value must be greater than 0"))
	}

	// get allowed values for order from context, if nothing allow all values
	if okOrder, ok := ctx.Value(appctx.PaginationOrderOptionsCTXKey).(map[string]string); ok {
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

	if q.Get("page") == "" {
		p.Page = 0
	} else {
		p.Page, err = strconv.Atoi(q.Get("page"))
		if err != nil {
			return fmt.Errorf("failed to parse pagination page parameter: %w", err)
		}
	}

	if q.Get("items") == "" {
		p.Items = 10
	} else {
		p.Items, err = strconv.Atoi(q.Get("items"))
		if err != nil {
			return fmt.Errorf("failed to parse pagination items parameter: %w", err)
		}
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
			} else if string(Descending) == strings.ToUpper(parts[1]) {
				po.Direction = Descending
			} else {
				return fmt.Errorf("failed to parse order direction: %s", strings.ToUpper(parts[1]))
			}
		}
		p.Order = append(p.Order, po)
	}
	p.RawOrder = q["order"]

	return nil
}

var (
	jsonTagRE = regexp.MustCompile(`json:"(.*?)"`)
	dbTagRE   = regexp.MustCompile(`db:"(.*?)"`)
)

// NewPagination - create a new Pagination struct and populate from url and order options
func NewPagination(ctx context.Context, url string, v interface{}) (context.Context, *Pagination, error) {
	var (
		pagination = new(Pagination)
		order      = map[string]string{}
	)

	// for the number of fields the struct v has
	for i := 0; i < reflect.TypeOf(v).Elem().NumField(); i++ {
		// get the struct tags to produce a mapping of json -> db
		tag := string(reflect.TypeOf(v).Elem().Field(i).Tag)

		// if we do have a tag
		if tag != "" {
			var (
				k string
				v string
			)

			// find the json tag
			jsonMatch := jsonTagRE.FindStringSubmatch(tag)
			if len(jsonMatch) > 1 {
				k = strings.Split(jsonMatch[1], ",")[0]
			}
			dbMatch := dbTagRE.FindStringSubmatch(tag)
			if len(dbMatch) > 1 {
				v = strings.Split(dbMatch[1], ",")[0]
			}

			if k != "" && v != "" {
				order[k] = v
			}
		}
	}

	ctx = context.WithValue(ctx, appctx.PaginationOrderOptionsCTXKey, order)

	if err := DecodeAndValidate(ctx, pagination, []byte(url)); err != nil {
		var (
			veParam = map[string]interface{}{}
			message = err.Error()
			me      *errorutils.MultiError
		)
		if errors.As(err, &me) {
			veParam["pagination"] = me.Errs
		}
		return ctx, nil, handlers.ValidationError(message, veParam)
	}
	return ctx, pagination, nil
}
