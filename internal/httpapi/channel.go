package httpapi

import (
	"net/http"

	"github.com/layer-3/nitrolite-go-example/internal/service"
)

func channelHandler(channelService *service.ChannelService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, err := requireQuery(r, "asset")
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		channel, err := channelService.GetHomeChannel(r.Context(), asset)
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"channel": encodeChannel(channel),
		})
	}
}

func channelStateHandler(channelService *service.ChannelService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		asset, err := requireQuery(r, "asset")
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		onlySigned, err := optionalBoolQuery(r, "only_signed", false)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		state, err := channelService.GetLatestState(r.Context(), asset, onlySigned)
		if err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"state": encodeState(state),
		})
	}
}
