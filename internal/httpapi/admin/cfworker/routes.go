package cfworker

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Post("/cf-worker/deploy", h.deploy)
	r.Get("/cf-worker/status", h.status)
	r.Delete("/cf-worker", h.deleteWorker)
}
