package wallet

import (
	"net/http"

	"github.com/brave-intl/bat-go/libs/middleware"
)

func noOpInstrumentHandler() middleware.InstrumentHandlerDef {
	return func(name string, h http.Handler) http.Handler {
		return h
	}
}
