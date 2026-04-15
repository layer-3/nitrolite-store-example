package httpapi

import (
	"net/http"

	"github.com/layer-3/nitrolite-go-example/internal/service"
)

func demoOverviewHandler(demoService *service.DemoService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		overview, err := demoService.GetOverview(r.Context(), r.URL.Query().Get("asset"))
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, encodeDemoOverview(overview))
	}
}
